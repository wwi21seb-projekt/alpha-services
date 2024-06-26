package dto

import imagev1 "github.com/wwi21seb-projekt/alpha-shared/gen/server_alpha/image/v1"

// ======================================== //
// =========== Shared DTOs ================ //
// ======================================== //

type Picture struct {
	URL    string `json:"url"`
	Width  int32  `json:"width"`
	Height int32  `json:"height"`
}

func TransformProtoPicToDTO(picture *imagev1.Picture) *Picture {
	if picture != nil {
		return &Picture{
			URL:    picture.GetUrl(),
			Width:  picture.GetWidth(),
			Height: picture.GetHeight(),
		}
	}
	return nil
}
