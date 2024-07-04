package schema

type Author struct {
	Username string   `json:"username"`
	Nickname string   `json:"nickname"`
	Picture  *Picture `json:"picture"`
}

type UserSubscription struct {
	FollowerId  string   `json:"followerId"`
	FollowingId string   `json:"followingId"`
	Username    string   `json:"username"`
	Nickname    string   `json:"nickname"`
	Picture     *Picture `json:"picture"`
}

// ----------------- Request Schemas -----------------

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
	Picture     *string `json:"picture" validate:"omitempty"`
}

type ChangePasswordRequest struct {
	OldPassword string `json:"oldPassword" validate:"required,min=8,password_validation"`
	NewPassword string `json:"newPassword" validate:"required,min=8,password_validation"`
}

type SetPasswordRequest struct {
	NewPassword string `json:"newPassword" validate:"required,min=8,password_validation"`
	Token       string `json:"token" validate:"required,numeric,len=6"`
}

// ----------------- Response Schemas -----------------

type TokenPairResponse struct {
	Token        string `json:"token"`
	RefreshToken string `json:"refreshToken"`
}

type GetUserResponse struct {
	Username       string   `json:"username"`
	Nickname       string   `json:"nickname"`
	Status         string   `json:"status"`
	Picture        *Picture `json:"picture"`
	FollowerCount  int32    `json:"follower"`
	FollowingCount int32    `json:"following"`
	PostCount      int32    `json:"posts"`
	SubscriptionId string   `json:"subscriptionId"`
}

type SearchUsersResponse struct {
	Users      []Author            `json:"records"`
	Pagination *PaginationResponse `json:"pagination"`
}

type ChangeTrivialInformamtionResponse struct {
	Nickname string `json:"nickname"`
	Status   string `json:"status"`
}

type CreateSubscriptionResponse struct {
	SubscriptionId    string `json:"subscriptionId"`
	SubscriptionDate  string `json:"subscriptionDate"`
	FollowerUsername  string `json:"follower"`
	FollowingUsername string `json:"following"`
}

type GetSubscriptionsResponse struct {
	Subscriptions []UserSubscription  `json:"records"`
	Pagination    *PaginationResponse `json:"pagination"`
}

type ResetPasswordResponse struct {
	Email string `json:"email"`
}
