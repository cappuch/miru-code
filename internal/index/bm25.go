package index

import (
	"encoding/json"
	"math"
	"os"

	"github.com/takara-ai/miru-code/internal/tokens"
)

const (
	bm25K1 = 1.5
	bm25B  = 0.75
)

// Posting is [docIndex, tf].
type Posting [2]int

// BM25Index is Okapi BM25 with inverted postings.
type BM25Index struct {
	docFreq     map[string]int
	docLengths  []int
	postings    map[string][]Posting
	avgDocLength float64
	numDocs     int
	totalLen    int
}

// NewBM25Index creates an empty index.
func NewBM25Index() *BM25Index {
	return &BM25Index{
		docFreq:  make(map[string]int),
		postings: make(map[string][]Posting),
	}
}

// AddDocument appends one tokenized document.
func (b *BM25Index) AddDocument(doc []string) int {
	docIndex := b.numDocs
	b.numDocs++
	b.docLengths = append(b.docLengths, len(doc))
	b.totalLen += len(doc)
	b.avgDocLength = float64(b.totalLen) / float64(b.numDocs)

	tf := make(map[string]int)
	for _, term := range doc {
		tf[term]++
	}
	for term, count := range tf {
		b.docFreq[term]++
		b.postings[term] = append(b.postings[term], Posting{docIndex, count})
	}
	return docIndex
}

// Index rebuilds from tokenized docs.
func (b *BM25Index) Index(tokenizedDocs [][]string) {
	b.numDocs = 0
	b.docFreq = make(map[string]int)
	b.docLengths = nil
	b.postings = make(map[string][]Posting)
	b.totalLen = 0
	b.avgDocLength = 0
	for _, doc := range tokenizedDocs {
		if doc == nil {
			continue
		}
		b.AddDocument(doc)
	}
}

type queryTerm struct {
	term string
	idf  float64
}

func (b *BM25Index) queryTerms(queryTokens []string) []queryTerm {
	seen := make(map[string]struct{})
	var terms []queryTerm
	for _, term := range queryTokens {
		if _, ok := seen[term]; ok {
			continue
		}
		seen[term] = struct{}{}
		df := b.docFreq[term]
		if df == 0 {
			continue
		}
		idf := math.Log(1 + (float64(b.numDocs-df)+0.5)/(float64(df)+0.5))
		terms = append(terms, queryTerm{term: term, idf: idf})
	}
	return terms
}

// GetScores returns BM25 scores for all docs.
func (b *BM25Index) GetScores(queryTokens []string, weightMask []bool) []float64 {
	scores := make([]float64, b.numDocs)
	if b.numDocs == 0 || len(queryTokens) == 0 {
		return scores
	}
	terms := b.queryTerms(queryTokens)
	avg := b.avgDocLength
	if avg == 0 {
		avg = 1
	}
	for _, t := range terms {
		list := b.postings[t.term]
		for _, p := range list {
			docIndex, tf := p[0], p[1]
			if weightMask != nil && (docIndex >= len(weightMask) || !weightMask[docIndex]) {
				continue
			}
			if docIndex >= len(b.docLengths) {
				continue
			}
			dl := float64(b.docLengths[docIndex])
			denom := float64(tf) + bm25K1*(1-bm25B+(bm25B*dl)/avg)
			scores[docIndex] += t.idf * ((float64(tf) * (bm25K1 + 1)) / denom)
		}
	}
	return scores
}

// GetScoresAsync is currently synchronous (goroutine parallelism optional later).
func (b *BM25Index) GetScoresAsync(queryTokens []string, weightMask []bool) []float64 {
	return b.GetScores(queryTokens, weightMask)
}

// NumDocs returns document count.
func (b *BM25Index) NumDocs() int { return b.numDocs }

// BM25JSON is the on-disk format.
type BM25JSON struct {
	Postings     map[string][]Posting `json:"postings"`
	DocLengths   []int                `json:"docLengths"`
	AvgDocLength float64              `json:"avgDocLength"`
	NumDocs      int                  `json:"numDocs"`
	// Legacy
	Docs [][]string `json:"docs,omitempty"`
}

// ToJSON serializes postings.
func (b *BM25Index) ToJSON() BM25JSON {
	postings := make(map[string][]Posting, len(b.postings))
	for term, list := range b.postings {
		postings[term] = list
	}
	return BM25JSON{
		Postings:     postings,
		DocLengths:   append([]int(nil), b.docLengths...),
		AvgDocLength: b.avgDocLength,
		NumDocs:      b.numDocs,
	}
}

// BM25FromJSON restores an index.
func BM25FromJSON(data BM25JSON) *BM25Index {
	idx := NewBM25Index()
	if data.Postings != nil {
		idx.docLengths = append([]int(nil), data.DocLengths...)
		idx.avgDocLength = data.AvgDocLength
		idx.numDocs = data.NumDocs
		idx.totalLen = int(data.AvgDocLength * float64(data.NumDocs))
		for term, list := range data.Postings {
			idx.postings[term] = list
			idx.docFreq[term] = len(list)
		}
		return idx
	}
	if data.Docs != nil {
		idx.Index(data.Docs)
	}
	return idx
}

// SaveBM25 writes bm25_index.json.
func SaveBM25(index *BM25Index, path string) error {
	data, err := json.Marshal(index.ToJSON())
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// LoadBM25 loads bm25_index.json.
func LoadBM25(path string) (*BM25Index, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var data BM25JSON
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, err
	}
	return BM25FromJSON(data), nil
}

// EnrichForBm25 matches sparse.ts / create.ts enrichment:
// content + filename stem×2 + last 3 path dirs.
func EnrichForBm25(content, filePath string) string {
	// ported from sparse.ts enrichForBm25
	return enrichForBm25(content, filePath)
}

// AddChunkToBm25 tokenizes enriched chunk text and adds it.
func AddChunkToBm25(b *BM25Index, content, filePath string) {
	b.AddDocument(tokens.Tokenize(EnrichForBm25(content, filePath)))
}
