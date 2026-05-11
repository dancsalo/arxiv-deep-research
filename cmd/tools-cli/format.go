package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/template"
)

// Stub type definitions for compilation
type SearchResult struct {
	Title           string
	Authors         string
	PublicationDate string
	URL             string
	Abstract        string
}

type PdfResult struct {
	Title         string
	SourceURL     string
	LocalPath     string
	PageCount     int
	ExtractedText string
}

type GithubSearchResult struct {
	FullName      string
	Description   string
	Language      string
	StarCount     int
	URL           string
	TopicTags     []string
}

type OutputFormat string

const (
	FormatHuman OutputFormat = "human"
	FormatJSON  OutputFormat = "json"
)

type FormatterOptions struct {
	Format      OutputFormat
	Writer      io.Writer
	ToolName    string
	RawResults  interface{}
	PrettyPrint bool
}

func formatOutput(opts FormatterOptions) error {
	switch opts.ToolName {
	case "search-arxiv", "search-openalex":
		return formatSearchResults(opts)
	case "fetch-pdf":
		return formatPdfResult(opts)
	case "search-github":
		return formatGithubResults(opts)
	default:
		return fmt.Errorf("unsupported tool: %s", opts.ToolName)
	}
}

func formatSearchResults(opts FormatterOptions) error {
	// Cast to the expected type for search results
	results, ok := opts.RawResults.([]SearchResult)
	if !ok {
		return fmt.Errorf("invalid results type for search tool")
	}

	switch opts.Format {
	case FormatHuman:
		tmpl := template.Must(template.New("search").Parse(`
{{- range .}}
Title: {{.Title}}
Authors: {{.Authors}}
Published: {{.PublicationDate}}
URL: {{.URL}}
Abstract: {{.Abstract | truncate 250}}

{{end -}}`))

		funcMap := template.FuncMap{
			"truncate": func(s string, length int) string {
				if len(s) <= length {
					return s
				}
				return s[:length] + "..."
			},
		}
		tmpl.Funcs(funcMap)

		var buf bytes.Buffer
		if err := tmpl.Execute(&buf, results); err != nil {
			return fmt.Errorf("error formatting human output: %w", err)
		}
		_, err := opts.Writer.Write(buf.Bytes())
		return err

	case FormatJSON:
		var jsonOutput []byte
		var err error
		if opts.PrettyPrint {
			jsonOutput, err = json.MarshalIndent(results, "", "  ")
		} else {
			jsonOutput, err = json.Marshal(results)
		}
		if err != nil {
			return fmt.Errorf("error marshaling JSON: %w", err)
		}
		_, err = opts.Writer.Write(jsonOutput)
		return err

	default:
		return fmt.Errorf("unsupported output format: %s", opts.Format)
	}
}

func formatPdfResult(opts FormatterOptions) error {
	// Cast to the expected type for PDF result
	result, ok := opts.RawResults.(types.PdfResult)
	if !ok {
		return fmt.Errorf("invalid results type for PDF tool")
	}

	switch opts.Format {
	case FormatHuman:
		tmpl := template.Must(template.New("pdf").Parse(`
PDF Details:
Title: {{.Title}}
Source URL: {{.SourceURL}}
Local Path: {{.LocalPath}}
Pages: {{.PageCount}}
Extracted Text Length: {{len .ExtractedText}} characters

Text Preview:
{{.ExtractedText | truncate 500}}
`))

		funcMap := template.FuncMap{
			"truncate": func(s string, length int) string {
				if len(s) <= length {
					return s
				}
				return s[:length] + "..."
			},
		}
		tmpl.Funcs(funcMap)

		var buf bytes.Buffer
		if err := tmpl.Execute(&buf, result); err != nil {
			return fmt.Errorf("error formatting human output: %w", err)
		}
		_, err := opts.Writer.Write(buf.Bytes())
		return err

	case FormatJSON:
		var jsonOutput []byte
		var err error
		if opts.PrettyPrint {
			jsonOutput, err = json.MarshalIndent(result, "", "  ")
		} else {
			jsonOutput, err = json.Marshal(result)
		}
		if err != nil {
			return fmt.Errorf("error marshaling JSON: %w", err)
		}
		_, err = opts.Writer.Write(jsonOutput)
		return err

	default:
		return fmt.Errorf("unsupported output format: %s", opts.Format)
	}
}

func formatGithubResults(opts FormatterOptions) error {
	// Cast to the expected type for GitHub search results
	results, ok := opts.RawResults.([]types.GithubSearchResult)
	if !ok {
		return fmt.Errorf("invalid results type for GitHub search tool")
	}

	switch opts.Format {
	case FormatHuman:
		tmpl := template.Must(template.New("github").Parse(`
{{- range .}}
Repository: {{.FullName}}
Description: {{.Description}}
Language: {{.Language}}
Stars: {{.StarCount}}
URL: {{.URL}}
{{- if .TopicTags}}
Topics: {{.TopicTags | join ", "}}
{{- end}}

{{end -}}`))

		funcMap := template.FuncMap{
			"join": func(elems []string, sep string) string {
				return strings.Join(elems, sep)
			},
		}
		tmpl.Funcs(funcMap)

		var buf bytes.Buffer
		if err := tmpl.Execute(&buf, results); err != nil {
			return fmt.Errorf("error formatting human output: %w", err)
		}
		_, err := opts.Writer.Write(buf.Bytes())
		return err

	case FormatJSON:
		var jsonOutput []byte
		var err error
		if opts.PrettyPrint {
			jsonOutput, err = json.MarshalIndent(results, "", "  ")
		} else {
			jsonOutput, err = json.Marshal(results)
		}
		if err != nil {
			return fmt.Errorf("error marshaling JSON: %w", err)
		}
		_, err = opts.Writer.Write(jsonOutput)
		return err

	default:
		return fmt.Errorf("unsupported output format: %s", opts.Format)
	}
}