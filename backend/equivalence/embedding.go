package equivalence

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"time"
)

const (
	embeddingModel  = "text-embedding-3-small"
	embeddingDim    = 1536
	maxBatchSize    = 100
	embeddingTimeout = 30 * time.Second
)

// EmbeddingClient calls OpenAI to generate text embeddings with caching.
type EmbeddingClient struct {
	apiKey  string
	baseURL string
	db      *sql.DB
	client  *http.Client
}

func NewEmbeddingClient(apiKey string, db *sql.DB) *EmbeddingClient {
	return &EmbeddingClient{
		apiKey:  apiKey,
		baseURL: "https://api.openai.com/v1",
		db:      db,
		client:  &http.Client{Timeout: embeddingTimeout},
	}
}

// SetBaseURL allows injecting a test server URL.
func (ec *EmbeddingClient) SetBaseURL(url string) {
	ec.baseURL = url
}

// Available returns true if the embedding client has a valid API key.
func (ec *EmbeddingClient) Available() bool {
	return ec.apiKey != ""
}

// GetEmbeddings returns embeddings for the given texts, using cache where available.
func (ec *EmbeddingClient) GetEmbeddings(ctx context.Context, texts []string) ([][]float64, error) {
	result := make([][]float64, len(texts))
	var uncached []int

	for i, text := range texts {
		hash := hashText(text)
		if emb, err := ec.loadCached(hash); err == nil {
			result[i] = emb
		} else {
			uncached = append(uncached, i)
		}
	}

	if len(uncached) == 0 {
		return result, nil
	}

	for batchStart := 0; batchStart < len(uncached); batchStart += maxBatchSize {
		batchEnd := batchStart + maxBatchSize
		if batchEnd > len(uncached) {
			batchEnd = len(uncached)
		}

		batchIndices := uncached[batchStart:batchEnd]
		batchTexts := make([]string, len(batchIndices))
		for j, idx := range batchIndices {
			batchTexts[j] = texts[idx]
		}

		embeddings, err := ec.callAPI(ctx, batchTexts)
		if err != nil {
			return nil, err
		}

		for j, idx := range batchIndices {
			result[idx] = embeddings[j]
			hash := hashText(texts[idx])
			if cacheErr := ec.saveCache(hash, embeddings[j]); cacheErr != nil {
				slog.Warn("failed to cache embedding", "error", cacheErr)
			}
		}
	}

	return result, nil
}

// CosineSimilarity computes the cosine similarity between two vectors.
func CosineSimilarity(a, b []float64) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0.0
	}

	var dot, magA, magB float64
	for i := range a {
		dot += a[i] * b[i]
		magA += a[i] * a[i]
		magB += b[i] * b[i]
	}

	if magA == 0 || magB == 0 {
		return 0.0
	}

	return dot / (math.Sqrt(magA) * math.Sqrt(magB))
}

func (ec *EmbeddingClient) callAPI(ctx context.Context, texts []string) ([][]float64, error) {
	body := map[string]interface{}{
		"model": embeddingModel,
		"input": texts,
	}
	bodyBytes, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(ctx, "POST", ec.baseURL+"/embeddings", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+ec.apiKey)

	resp, err := ec.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("embedding API request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("embedding API returned %d", resp.StatusCode)
	}

	var apiResp embeddingAPIResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("decode embedding response: %w", err)
	}

	result := make([][]float64, len(texts))
	for _, item := range apiResp.Data {
		if item.Index < len(result) {
			result[item.Index] = item.Embedding
		}
	}

	return result, nil
}

func (ec *EmbeddingClient) loadCached(hash string) ([]float64, error) {
	if ec.db == nil {
		return nil, fmt.Errorf("no database")
	}

	var blob []byte
	err := ec.db.QueryRow("SELECT embedding FROM embedding_cache WHERE title_hash = ?", hash).Scan(&blob)
	if err != nil {
		return nil, err
	}

	return decodeEmbedding(blob)
}

func (ec *EmbeddingClient) saveCache(hash string, embedding []float64) error {
	if ec.db == nil {
		return nil
	}

	blob := encodeEmbedding(embedding)
	_, err := ec.db.Exec(
		"INSERT OR REPLACE INTO embedding_cache (title_hash, embedding, model) VALUES (?, ?, ?)",
		hash, blob, embeddingModel,
	)
	return err
}

func hashText(text string) string {
	h := sha256.Sum256([]byte(text))
	return fmt.Sprintf("%x", h)
}

func encodeEmbedding(emb []float64) []byte {
	buf := new(bytes.Buffer)
	for _, v := range emb {
		binary.Write(buf, binary.LittleEndian, v)
	}
	return buf.Bytes()
}

func decodeEmbedding(data []byte) ([]float64, error) {
	n := len(data) / 8
	result := make([]float64, n)
	reader := bytes.NewReader(data)
	for i := 0; i < n; i++ {
		if err := binary.Read(reader, binary.LittleEndian, &result[i]); err != nil {
			return nil, err
		}
	}
	return result, nil
}

type embeddingAPIResponse struct {
	Data []embeddingItem `json:"data"`
}

type embeddingItem struct {
	Index     int       `json:"index"`
	Embedding []float64 `json:"embedding"`
}
