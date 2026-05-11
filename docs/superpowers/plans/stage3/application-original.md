============
High-level overview
============

Stack:
 - Use the Anthropic SDK in Go. 
 - Langfuse in docker-compose

Requiements:
 - stops iterating after _n_ iterations or _X_ dollars
 - Make sure the agent loop can handle multiple requests concurrently. 
 - Make sure the agent loop can call multiple tools or skills concurrently.
 - When considering a skill or tool
 - Log all traces into langfuse.

input: natural language topic or question from user

output:
 - high-impact authors or thought leaders that speak on this topic, and a quick bio and summary of their work.
 - most cited works for that particular topic, recommended reading with summaries. "seminal papers" or "influential blog posts". 
 - collective summary different perspectives on the question OR the different active areas of research on the topic.
 - suggest follow-on deeper dive topics "suggestions for what to search for next" 

ASK: Create an architecture for this loop based on claude code. 


assets: postgres database with previous completed runs.
use the one that langfuse uses, just a different table.

Agent actions (mix of skills and tools)
1. General Web search. Use topic (or a reformulation of it) to do a web search to return webpages & determine which blogs are actually relevant. Store the relevant blogs full text in vectordb alongisde metadata (such as date, author, (generated) keywords); vector is the ai generated summary of the blog.
2. Targeted web search. Optionally search the various high-impact sites listed below: develop connects for each of them and how to query. 
3. Arxiv search. Reformulation of topic/question into arxiv query & Execution of arxiv query to grab list of paper metadata. decide which paper metadata are actually relevant to the topic & Pull full text and citation count from openalex API. Store those papers full text in vectordb, with arxiv metadata and openalex metadata; vector is the ai generated summary, vectorized.
4. Pull previous relevant searches and/or from memory.

ASK #1:




 Most practical for this project:                                
   
  1. HN Algolia API — GET                                         
  https://hn.algolia.com/api/v1/search?query=...&tags=story
  returns stories with points, comment counts, and URLs. High     
  points = community-validated. Free, no auth, fast.    
  2. Targeted site: searches — add specific site-scoped queries to
   the existing DDG web search step:                              
    - site:substack.com {topic}
    - site:lilianweng.github.io {topic}                           
    - site:huggingface.co/blog {topic}                            
  3. RSS feeds from curated newsletter list — if you maintain a   
  list of 10-20 high-quality ML newsletters/blogs, you can search 
  their RSS feeds directly.                                       
                                                                  
  The site: approach is the lowest-effort win — it works with the 
  existing DDG search in example.py and just needs better query
  construction in the follow-on search step.                      
                                              


OpenAlex returns a rich set of fields per work that would be    
  useful for this project:                                      
                                                                  
  Ranking / importance signals:                                   
  - cited_by_count — total citations                              
  - counts_by_year — citation trajectory (growing vs declining)   
  - is_retracted — flag retracted papers                        
  - is_oa — open access status (helpful for PDF availability)     
                                                                  
  Richer metadata than arXiv provides:                            
  - concepts — hierarchical topic tags with relevance scores      
  (e.g., {"display_name": "Reinforcement learning", "score":      
  0.87})                                                          
  - topics — newer, more granular topic classification            
  - keywords — author-provided keywords                         
  - type — article, review, preprint, etc. (useful for filtering  
  surveys)                                                        
  - referenced_works — outgoing citation list (paper IDs it cites)
  - related_works — algorithmically similar papers                
                                                                  
  Author enrichment:                                              
  - authorships[].author.id — persistent author IDs               
  - authorships[].institutions — affiliations at time of          
  publication                                                     
  - authorships[].author.orcid                                    
                                                                
  Venue info:                                                     
  - primary_location.source.display_name — journal/conference name
  - primary_location.source.type — journal, conference, repository
                                                                  
  For this project specifically, the most valuable additions      
  beyond citation count would be:                                 
  - concepts / topics — better categorization than arXiv          
  categories alone                                                
  - referenced_works — follow citation chains to find foundational
   papers                                                         
  - related_works — discover papers arXiv search missed           
  - counts_by_year — prioritize papers with growing citation    
  momentum                                                        
             

Researcher blogs:                                               
  - Lilian Weng — lilianweng.github.io (OpenAI, exceptional     
  survey-style posts)                                             
  - Sebastian Raschka — sebastianraschka.com (LLM practitioner    
  insights)                                                       
  - Jay Alammar — jalammar.github.io (visual explanations)        
  - Chip Huyen — huyenchip.com (MLOps, systems)           
  - Sasha Rush — rush-nlp.com (Cornell/Hugging Face)              
                                                                  
  Substacks:                                                      
  - The Gradient — thegradient.pub                                
  - Interconnects (Nathan Lambert) — interconnects.ai (RLHF,      
  alignment)                                                      
  - Ahead of AI (Sebastian Raschka) —                             
  magazine.sebastianraschka.com                         
  - The Batch (Andrew Ng) — deeplearning.ai/the-batch             
  - Davis Summarizes Papers — dblalock.substack.com     
  - Latent Space — latent.space (ML engineering)                  
  - The Pragmatic Engineer (tangential) — less ML-specific but    
  covers AI infra                                                 
                                                                  
  Company/org blogs:                                              
  - Hugging Face — huggingface.co/blog                            
  - OpenAI — openai.com/research                                  
  - Google DeepMind — deepmind.google/discover/blog               
  - Anthropic — anthropic.com/research                            
  - Meta AI (FAIR) — ai.meta.com/blog                             
  - Microsoft Research — microsoft.com/en-us/research/blog        
  - Eleuther AI — blog.eleuther.ai                                
  - Weights & Biases — wandb.ai/fully-connected                   
                                                                  
  Community/aggregator:                                           
  - Distill.pub (inactive but still excellent reference)          
  - ML Commons — mlcommons.org                                    
  - Papers With Code blog — paperswithcode.com                    
  - Import AI (Jack Clark) — importai.net                         
                                                                  
  For the project, a curated list of ~15 high-signal domains to   
  use as site: targets in the follow-on web search step would     
  cover most of the quality ML commentary.                        
                                             