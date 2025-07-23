package dto

type RegisterRequest struct {
	Username string `json:"username" binding:"required,min=3,max=50"`
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=8,max=20"`
}

type RegisterResponse struct {
	Uid            string `json:"uid"`
	Token          string `json:"token"`
	TokenExpiresAt string `json:"tokenExpiresAt"`
}

type LoginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}
