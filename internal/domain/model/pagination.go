package model

type SortOrder string

const (
	SortAsc  SortOrder = "ASC"
	SortDesc SortOrder = "DESC"
)

type Pagination struct {
	Limit int
	Page  int
	Sort  string
}

type Page[T any] struct {
	Items   []T  `json:"items"`
	HasNext bool `json:"has_next"`
	Total   *int `json:"total,omitempty"`
}

type SortField struct {
	Field string
	Order SortOrder
}
