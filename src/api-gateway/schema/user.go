package schema

type Author struct {
	Username          string `json:"username"`
	Nickname          string `json:"nickname"`
	ProfilePictureUrl string `json:"profilePictureURL"`
}

// ----------------- Request Schemas -----------------

type RegistrationRequest struct {
	Username string `json:"username" validate:"required,max=20,username_validation"`
	Nickname string `json:"nickname" validate:"max=25"`
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required,min=8,password_validation"`
}

type LoginRequest struct {
	Username string `json:"username" validate:"required,max=20,username_validation"`
	Password string `json:"password" validate:"required,min=8,password_validation"`
}

type ActivationRequest struct {
	Token string `json:"token" validate:"required,numeric,len=6"`
}

type RefreshTokenRequest struct {
	RefreshToken string `json:"refreshToken" validate:"required"`
}

type SubscriptionRequest struct {
	Following string `json:"following" validate:"required,max=20,username_validation"`
}

type ChangeTrivialInformationRequest struct {
	NewNickname string `json:"nickname" validate:"max=25"`
	Status      string `json:"status" validate:"max=256"`
}

type ChangePasswordRequest struct {
	OldPassword string `json:"oldPassword" validate:"required,min=8,password_validation"`
	NewPassword string `json:"newPassword" validate:"required,min=8,password_validation"`
}
