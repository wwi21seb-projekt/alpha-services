package dto

import (
	"fmt"
	userv1 "github.com/wwi21seb-projekt/alpha-shared/gen/server_alpha/user/v1"
)

// ======================================== //
// =========== Shared DTOs ================ //
// ======================================== //

type User struct {
	Username string   `json:"username"`
	Nickname string   `json:"nickname"`
	Picture  *Picture `json:"picture"`
}

// ======================================== //
// =========== Request DTOs =============== //
// ======================================== //

type RegistrationRequest struct {
	Username string `json:"username" validate:"required,max=20,username_validation"`
	Nickname string `json:"nickname" validate:"max=25"`
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required,min=8,password_validation"`
	Picture  string `json:"profilePicture"`
}

type LoginRequest struct {
	Username string `json:"username" validate:"required,max=20,username_validation"`
	Password string `json:"password" validate:"required,min=8,password_validation"`
}

// ======================================== //
// =========== Response DTOs ============== //
// ======================================== //

type RegistrationResponse struct {
	Username string   `json:"username"`
	Nickname string   `json:"nickname"`
	Email    string   `json:"email"`
	Picture  *Picture `json:"picture"`
}

type TokenPairResponse struct {
	Token        string `json:"token"`
	RefreshToken string `json:"refreshToken"`
}

// ======================================== //
// ========== Helper Functions ============ //
// ======================================== //

func TransformProtoUserToDTO(user *userv1.User) User {
	if user == nil {
		fmt.Printf("User is nil")
		return User{}
	}
	return User{
		Username: user.GetUsername(),
		Nickname: user.GetNickname(),
		Picture:  TransformProtoPicToDTO(user.Picture),
	}
}
