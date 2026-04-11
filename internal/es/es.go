package es

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"strings"
	"supply-chain/internal/config"
	"supply-chain/internal/model"
	"time"

	"github.com/elastic/go-elasticsearch/v8"
	"github.com/elastic/go-elasticsearch/v8/esapi"
)

var Client *elasticsearch.Client

func InitES(cfg *config.ElasticsearchConfig) error {
	esCfg := elasticsearch.Config{
		Addresses: cfg.Addresses,
		Username:  cfg.Username,
		Password:  cfg.Password,
	}
	client, err := elasticsearch.NewClient(esCfg)
	if err != nil {
		return fmt.Errorf("failed to create ES client: %w", err)
	}
	Client = client

	res, err := client.Info()
	if err != nil {
		return fmt.Errorf("failed to connect to ES: %w", err)
	}
	defer res.Body.Close()
	log.Println("[ES] Connected to Elasticsearch")
	return nil
}

// ProductDocument is the ES document structure for products.
type ProductDocument struct {
	ID           uint64   `json:"id"`
	Name         string   `json:"name"`
	ProductCode  string   `json:"product_code"`
	Supplier     string   `json:"supplier"`
	Tags         []string `json:"tags"`
	Status       uint8    `json:"status"`
	ImageURL     string   `json:"image_url"`
	Brand        string   `json:"brand"`
	Category     string   `json:"category"`
	Material     string   `json:"material"`
	PatentStatus string   `json:"patent_status"`
	FactoryPrice float64  `json:"factory_price"`
	CreatedAt    string   `json:"created_at"`
}

func ProductToDocument(p *model.Product) ProductDocument {
	tags := []string(p.Tags)
	if tags == nil {
		tags = []string{}
	}
	return ProductDocument{
		ID:           p.ID,
		Name:         p.Name,
		ProductCode:  p.ProductCode,
		Supplier:     p.Supplier,
		Tags:         tags,
		Status:       p.Status,
		ImageURL:     p.ImageURL,
		Brand:        p.Brand,
		Category:     p.Category,
		Material:     p.Material,
		PatentStatus: p.PatentStatus,
		FactoryPrice: p.FactoryPrice,
		CreatedAt:    p.CreatedAt.Format(time.RFC3339),
	}
}

// IndexProduct indexes or updates a single product document in ES.
func IndexProduct(ctx context.Context, index string, p *model.Product) error {
	doc := ProductToDocument(p)
	data, err := json.Marshal(doc)
	if err != nil {
		return err
	}

	req := esapi.IndexRequest{
		Index:      index,
		DocumentID: fmt.Sprintf("%d", p.ID),
		Body:       bytes.NewReader(data),
		Refresh:    "false",
	}
	res, err := req.Do(ctx, Client)
	if err != nil {
		return fmt.Errorf("ES index error: %w", err)
	}
	defer res.Body.Close()
	if res.IsError() {
		body, _ := io.ReadAll(res.Body)
		return fmt.Errorf("ES index error [%s]: %s", res.Status(), string(body))
	}
	return nil
}

// DeleteProduct removes a product document from ES.
func DeleteProduct(ctx context.Context, index string, productID uint64) error {
	req := esapi.DeleteRequest{
		Index:      index,
		DocumentID: fmt.Sprintf("%d", productID),
		Refresh:    "false",
	}
	res, err := req.Do(ctx, Client)
	if err != nil {
		return fmt.Errorf("ES delete error: %w", err)
	}
	defer res.Body.Close()
	if res.IsError() && res.StatusCode != 404 {
		body, _ := io.ReadAll(res.Body)
		return fmt.Errorf("ES delete error [%s]: %s", res.Status(), string(body))
	}
	return nil
}

// SearchResult holds the ES search response data.
type SearchResult struct {
	IDs             []uint64
	Total           int64
	SearchAfterSort []interface{}
}

// SearchProducts performs a product search with search_after pagination.
// Sorted by product_code asc, id asc for consistent ordering.
// Case-insensitive wildcard on product_code.
// Supports multi-supplier and multi-tag filters.
// ScopeSuppliers/ScopeTags are employee access restrictions (injected by service layer).
func SearchProducts(ctx context.Context, index string, req *model.ProductListReq) (*SearchResult, error) {
	must := []map[string]interface{}{}
	filter := []map[string]interface{}{}

	// Case-insensitive wildcard search on product_code
	if req.ProductCode != "" {
		must = append(must, map[string]interface{}{
			"wildcard": map[string]interface{}{
				"product_code.keyword": map[string]interface{}{
					"value":            strings.ToLower(req.ProductCode) + "*",
					"case_insensitive": true,
				},
			},
		})
	}

	// Prefix search on name (still exact-case as names are Chinese)
	if req.Name != "" {
		must = append(must, map[string]interface{}{
			"prefix": map[string]interface{}{
				"name.keyword": req.Name,
			},
		})
	}

	// Multi-supplier filter (OR among selected suppliers)
	if len(req.Suppliers) > 0 {
		filter = append(filter, map[string]interface{}{
			"terms": map[string]interface{}{
				"supplier.keyword": req.Suppliers,
			},
		})
	}

	// Multi-tag filter (product must have at least one of the selected tags)
	if len(req.Tags) > 0 {
		filter = append(filter, map[string]interface{}{
			"terms": map[string]interface{}{
				"tags": req.Tags,
			},
		})
	}

	// Status filter
	if req.Status != nil {
		filter = append(filter, map[string]interface{}{
			"term": map[string]interface{}{
				"status": *req.Status,
			},
		})
	}

	// Date range filter
	if req.StartDate != "" || req.EndDate != "" {
		rangeQ := map[string]interface{}{}
		if req.StartDate != "" {
			rangeQ["gte"] = req.StartDate
		}
		if req.EndDate != "" {
			rangeQ["lte"] = req.EndDate
		}
		filter = append(filter, map[string]interface{}{
			"range": map[string]interface{}{
				"created_at": rangeQ,
			},
		})
	}

	// Employee scope: restrict to allowed suppliers (AND with user filters)
	if len(req.ScopeSuppliers) > 0 {
		filter = append(filter, map[string]interface{}{
			"terms": map[string]interface{}{
				"supplier.keyword": req.ScopeSuppliers,
			},
		})
	}

	// Employee scope: restrict to allowed tags (intersection)
	if len(req.ScopeTags) > 0 {
		filter = append(filter, map[string]interface{}{
			"terms": map[string]interface{}{
				"tags": req.ScopeTags,
			},
		})
	}

	if len(must) == 0 {
		must = append(must, map[string]interface{}{
			"match_all": map[string]interface{}{},
		})
	}

	query := map[string]interface{}{
		"size": req.PageSize,
		"query": map[string]interface{}{
			"bool": map[string]interface{}{
				"must":   must,
				"filter": filter,
			},
		},
		// Sort by product_code asc for natural ordering, id asc for tie-breaking
		"sort": []map[string]interface{}{
			{"product_code.keyword": map[string]interface{}{"order": "asc"}},
			{"id": map[string]interface{}{"order": "asc"}},
		},
		"_source":          []string{"id"},
		"track_total_hits": true,
	}

	// search_after pagination
	if req.SearchAfterCode != "" && req.SearchAfterID != "" {
		query["search_after"] = []interface{}{req.SearchAfterCode, req.SearchAfterID}
	}

	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(query); err != nil {
		return nil, fmt.Errorf("failed to encode query: %w", err)
	}

	res, err := Client.Search(
		Client.Search.WithContext(ctx),
		Client.Search.WithIndex(index),
		Client.Search.WithBody(&buf),
	)
	if err != nil {
		return nil, fmt.Errorf("ES search error: %w", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		body, _ := io.ReadAll(res.Body)
		return nil, fmt.Errorf("ES search error [%s]: %s", res.Status(), string(body))
	}

	var result map[string]interface{}
	if err := json.NewDecoder(res.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode ES response: %w", err)
	}

	hits := result["hits"].(map[string]interface{})
	totalObj := hits["total"].(map[string]interface{})
	total := int64(totalObj["value"].(float64))

	hitList := hits["hits"].([]interface{})
	ids := make([]uint64, 0, len(hitList))
	var lastSort []interface{}

	for _, h := range hitList {
		hit := h.(map[string]interface{})
		source := hit["_source"].(map[string]interface{})
		id := uint64(source["id"].(float64))
		ids = append(ids, id)
		if sortVals, ok := hit["sort"].([]interface{}); ok {
			lastSort = sortVals
		}
	}

	return &SearchResult{
		IDs:             ids,
		Total:           total,
		SearchAfterSort: lastSort,
	}, nil
}

// BulkIndex indexes multiple product documents using the bulk API.
func BulkIndex(ctx context.Context, index string, products []model.Product) error {
	if len(products) == 0 {
		return nil
	}

	var buf bytes.Buffer
	for _, p := range products {
		meta := map[string]interface{}{
			"index": map[string]interface{}{
				"_index": index,
				"_id":    fmt.Sprintf("%d", p.ID),
			},
		}
		metaLine, _ := json.Marshal(meta)
		buf.Write(metaLine)
		buf.WriteByte('\n')

		doc := ProductToDocument(&p)
		docLine, _ := json.Marshal(doc)
		buf.Write(docLine)
		buf.WriteByte('\n')
	}

	res, err := Client.Bulk(
		bytes.NewReader(buf.Bytes()),
		Client.Bulk.WithContext(ctx),
		Client.Bulk.WithIndex(index),
	)
	if err != nil {
		return fmt.Errorf("ES bulk error: %w", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		body, _ := io.ReadAll(res.Body)
		return fmt.Errorf("ES bulk error [%s]: %s", res.Status(), string(body))
	}
	return nil
}

// CreateProductIndex creates the ES index with the proper mapping.
func CreateProductIndex(ctx context.Context, index string) error {
	mapping := `{
  "settings": {
    "number_of_shards": 3,
    "number_of_replicas": 1,
    "analysis": {
      "analyzer": {
        "ik_smart_analyzer": {
          "type": "custom",
          "tokenizer": "ik_smart"
        },
        "ik_max_word_analyzer": {
          "type": "custom",
          "tokenizer": "ik_max_word"
        }
      }
    }
  },
  "mappings": {
    "properties": {
      "id":            { "type": "long" },
      "name":          { "type": "text", "analyzer": "ik_max_word_analyzer", "search_analyzer": "ik_smart_analyzer", "fields": { "keyword": { "type": "keyword" } } },
      "product_code":  { "type": "text", "analyzer": "ik_max_word_analyzer", "search_analyzer": "ik_smart_analyzer", "fields": { "keyword": { "type": "keyword" } } },
      "supplier":      { "type": "text", "analyzer": "ik_max_word_analyzer", "search_analyzer": "ik_smart_analyzer", "fields": { "keyword": { "type": "keyword" } } },
      "tags":          { "type": "keyword" },
      "status":        { "type": "byte" },
      "image_url":     { "type": "keyword", "index": false },
      "brand":         { "type": "keyword" },
      "category":      { "type": "keyword" },
      "material":      { "type": "keyword" },
      "patent_status": { "type": "keyword" },
      "factory_price": { "type": "scaled_float", "scaling_factor": 100 },
      "created_at":    { "type": "date", "format": "strict_date_optional_time||epoch_millis" }
    }
  }
}`

	res, err := Client.Indices.Create(
		index,
		Client.Indices.Create.WithContext(ctx),
		Client.Indices.Create.WithBody(strings.NewReader(mapping)),
	)
	if err != nil {
		return fmt.Errorf("ES create index error: %w", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		body, _ := io.ReadAll(res.Body)
		return fmt.Errorf("ES create index error [%s]: %s", res.Status(), string(body))
	}
	log.Printf("[ES] Index '%s' created successfully\n", index)
	return nil
}

// DeleteIndex deletes an ES index.
func DeleteIndex(ctx context.Context, index string) error {
	res, err := Client.Indices.Delete(
		[]string{index},
		Client.Indices.Delete.WithContext(ctx),
	)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	return nil
}
