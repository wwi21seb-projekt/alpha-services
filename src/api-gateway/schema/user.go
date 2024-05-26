package schema

type Author struct {
	Username          string `json:"username"`
	Nickname          string `json:"nickname"`
	ProfilePictureUrl string `json:"profilePictureURL"`
}
