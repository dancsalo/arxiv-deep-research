package research

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

type ArxivResult struct {
	Title     string   `json:"title"`
	Authors   []string `json:"authors"`
	Abstract  string   `json:"abstract"`
	Link      string   `json:"link"`
	Published string   `json:"published"`
}

type atomFeed struct {
	XMLName xml.Name    `xml:"feed"`
	Entries []atomEntry `xml:"entry"`
}

type atomEntry struct {
	Title     string       `xml:"title"`
	Summary   string       `xml:"summary"`
	ID        string       `xml:"id"`
	Authors   []atomAuthor `xml:"author"`
	Published string       `xml:"published"`
}

type atomAuthor struct {
	Name string `xml:"name"`
}

func (r *ResearchToolSet) handleSearchArxiv(ctx context.Context, input json.RawMessage) (string, error) {
	var params struct {
		Query      string `json:"query"`
		MaxResults int    `json:"max_results"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return toolError("invalid input: "+err.Error(), false), nil
	}
	if params.MaxResults <= 0 {
		params.MaxResults = 10
	}

	u := fmt.Sprintf("https://export.arxiv.org/api/query?search_query=all:%s&max_results=%d",
		url.QueryEscape(params.Query), params.MaxResults)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return toolError("request creation failed: "+err.Error(), false), nil
	}

	resp, err := r.client.Do(req)
	if err != nil {
		return toolError("arXiv request failed: "+err.Error(), true), nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return toolError(fmt.Sprintf("arXiv returned status %d", resp.StatusCode), true), nil
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return toolError("failed to read response: "+err.Error(), true), nil
	}

	var feed atomFeed
	if err := xml.Unmarshal(body, &feed); err != nil {
		return toolError("failed to parse arXiv XML: "+err.Error(), true), nil
	}

	results := make([]ArxivResult, 0, len(feed.Entries))
	for _, entry := range feed.Entries {
		authors := make([]string, 0, len(entry.Authors))
		for _, a := range entry.Authors {
			authors = append(authors, a.Name)
		}
		results = append(results, ArxivResult{
			Title:     strings.TrimSpace(entry.Title),
			Authors:   authors,
			Abstract:  strings.TrimSpace(entry.Summary),
			Link:      strings.TrimSpace(entry.ID),
			Published: entry.Published,
		})
	}

	b, _ := json.Marshal(results)
	return string(b), nil
}

type OpenAlexResult struct {
	Title    string   `json:"title"`
	Authors  []string `json:"authors"`
	DOI      string   `json:"doi"`
	Abstract string   `json:"abstract"`
	Year     int      `json:"year"`
}

type openAlexResponse struct {
	Results []openAlexWork `json:"results"`
}

type openAlexWork struct {
	Title                 string               `json:"title"`
	DOI                   string               `json:"doi"`
	PublicationYear       int                  `json:"publication_year"`
	Authorships           []openAlexAuthorship `json:"authorships"`
	AbstractInvertedIndex map[string][]int     `json:"abstract_inverted_index"`
}

type openAlexAuthorship struct {
	Author struct {
		DisplayName string `json:"display_name"`
	} `json:"author"`
}

func (r *ResearchToolSet) handleSearchOpenAlex(ctx context.Context, input json.RawMessage) (string, error) {
	var params struct {
		Query      string `json:"query"`
		MaxResults int    `json:"max_results"`
		Filter     string `json:"filter"`
	}
	if err := json.Unmarshal(input, &params); err != nil {
		return toolError("invalid input: "+err.Error(), false), nil
	}
	if params.MaxResults <= 0 {
		params.MaxResults = 10
	}

	u := "https://api.openalex.org/works?search=" + url.QueryEscape(params.Query) +
		"&per_page=" + strconv.Itoa(params.MaxResults) +
		"&mailto=arxiv-deep-research@users.noreply.github.com"
	if params.Filter != "" {
		u += "&filter=" + url.QueryEscape(params.Filter)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return toolError("request creation failed: "+err.Error(), false), nil
	}

	resp, err := r.client.Do(req)
	if err != nil {
		return toolError("OpenAlex request failed: "+err.Error(), true), nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return toolError(fmt.Sprintf("OpenAlex returned status %d", resp.StatusCode), true), nil
	}

	var oaResp openAlexResponse
	if err := json.NewDecoder(resp.Body).Decode(&oaResp); err != nil {
		return toolError("failed to parse OpenAlex response: "+err.Error(), true), nil
	}

	results := make([]OpenAlexResult, 0, len(oaResp.Results))
	for _, work := range oaResp.Results {
		authors := make([]string, 0, len(work.Authorships))
		for _, a := range work.Authorships {
			authors = append(authors, a.Author.DisplayName)
		}
		results = append(results, OpenAlexResult{
			Title:    work.Title,
			Authors:  authors,
			DOI:      work.DOI,
			Abstract: reconstructAbstract(work.AbstractInvertedIndex),
			Year:     work.PublicationYear,
		})
	}

	b, _ := json.Marshal(results)
	return string(b), nil
}

func reconstructAbstract(index map[string][]int) string {
	if len(index) == 0 {
		return ""
	}

	maxPos := 0
	for _, positions := range index {
		for _, pos := range positions {
			if pos > maxPos {
				maxPos = pos
			}
		}
	}

	words := make([]string, maxPos+1)
	type wordPos struct {
		word string
		pos  int
	}
	var pairs []wordPos
	for word, positions := range index {
		for _, pos := range positions {
			pairs = append(pairs, wordPos{word, pos})
		}
	}
	sort.Slice(pairs, func(i, j int) bool { return pairs[i].pos < pairs[j].pos })
	for _, p := range pairs {
		words[p.pos] = p.word
	}

	return strings.Join(words, " ")
}
