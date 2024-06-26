package dto

import userv1 "github.com/wwi21seb-projekt/alpha-shared/gen/server_alpha/user/v1"

// ======================================== //
// =========== Shared DTOs ================ //
// ======================================== //

// ======================================== //
// =========== Response DTOs ============== //
// ======================================== //

// ======================================== //
// ========== Helper Functions ============ //
// ======================================== //

type User struct {
	Username string   `json:"username"`
	Nickname string   `json:"nickname"`
	Picture  *Picture `json:"picture"`
}

func TransformProtoUserToDTO(user *userv1.User) *User {
	if user == nil {
		return nil
	}
	return &User{
		Username: user.GetUsername(),
		Nickname: user.GetNickname(),
		Picture:  TransformProtoPicToDTO(user.Picture),
	}
}
