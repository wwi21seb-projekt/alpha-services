package schema

type PaginationResponse struct {
	Offset  int32 `json:"offset"`
	Limit   int32 `json:"limit"`
	Records int32 `json:"records"`
}
