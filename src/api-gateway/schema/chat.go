package schema

import "encoding/json"

type CreateChatRequest struct {
	Username string `json:"username" validate:"required,max=20,username_validation"`
	Content  string `json:"content" validate:"required,max=256,min=1,post_validation"`
}

type CreateChatResponse struct {
	ChatID  string  `json:"chatId"`
	Message Message `json:"message"`
}

type GetChatsResponse struct {
	Chats []ChatInformation `json:"records"`
}

type ChatInformation struct {
	ChatID string `json:"chatId"`
	User   Author `json:"user"`
}

type GetChatResponse struct {
	Messages   []Message          `json:"records"`
	Pagination PaginationResponse `json:"pagination"`
}

type Message struct {
	Username     string `json:"username"`
	Content      string `json:"content"`
	CreationDate string `json:"creationDate"`
}

func (m *Message) UnmarshalJSON(b []byte) error {
	type Alias Message
	aux := &struct {
		*Alias
	}{
		Alias: (*Alias)(m),
	}
	if err := json.Unmarshal(b, &aux); err != nil {
		return err
	}
	return nil
}

func (m Message) MarshalJSON() ([]byte, error) {
	type Alias Message
	return json.Marshal(&struct {
		*Alias
	}{
		Alias: (*Alias)(&m),
	})
}
