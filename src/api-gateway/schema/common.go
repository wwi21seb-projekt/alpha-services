package schema

type PaginationResponse struct {
	Offset  int64 `json:"offset"`
	Limit   int32 `json:"limit"`
	Records int32 `json:"records"`
}

type Picture struct {
	Url    string `json:"url"`
	Width  int32  `json:"width"`
	Height int32  `json:"height"`
}
