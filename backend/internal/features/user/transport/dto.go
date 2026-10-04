package transport

import "github.com/bharatchat/backend/internal/features/user/domain"

type UserResponse struct {
	ID                    string  `json:"id"`
	PhoneNumber           string  `json:"phoneNumber"`
	Username              *string `json:"username"`
	DisplayName           string  `json:"displayName"`
	AvatarURL             *string `json:"avatarUrl"`
	About                 string  `json:"about"`
	IsVerified            bool    `json:"isVerified"`
	IsOnline              bool    `json:"isOnline"`
	PrivacyLastSeen       string  `json:"privacyLastSeen"`
	PrivacyAvatar         string  `json:"privacyAvatar"`
	PrivacyAbout          string  `json:"privacyAbout"`
	PrivacyReadReceipts   bool    `json:"privacyReadReceipts"`
	PrivacyPhone          string  `json:"privacyPhone"`
	DiscoverableByPhone   bool    `json:"discoverableByPhone"`
	AllowGroupAdds        string  `json:"allowGroupAdds"`
	ShareTypingIndicators bool    `json:"shareTypingIndicators"`
	SecurityNotifications bool    `json:"securityNotifications"`
}

func ToUserResponse(u *domain.User) UserResponse {
	return UserResponse{ID: u.ID, PhoneNumber: u.PhoneNumber, Username: u.Username, DisplayName: u.DisplayName, AvatarURL: u.AvatarURL, About: u.About, IsVerified: u.IsVerified, IsOnline: u.IsOnline, PrivacyLastSeen: string(u.PrivacyLastSeen), PrivacyAvatar: string(u.PrivacyAvatar), PrivacyAbout: string(u.PrivacyAbout), PrivacyReadReceipts: u.PrivacyReadReceipts, PrivacyPhone: string(u.PrivacyPhone), DiscoverableByPhone: u.DiscoverableByPhone, AllowGroupAdds: string(u.AllowGroupAdds), ShareTypingIndicators: u.ShareTypingIndicators, SecurityNotifications: u.SecurityNotifications}
}

type UpdateProfileRequest struct {
	DisplayName *string `json:"displayName,omitempty" validate:"omitempty,min=1,max=80"`
	Username    *string `json:"username,omitempty" validate:"omitempty,alphanum,min=3,max=30"`
	About       *string `json:"about,omitempty" validate:"omitempty,max=200"`
	AvatarURL   *string `json:"avatarUrl,omitempty" validate:"omitempty,url"`
}
type UsernameAvailabilityResponse struct {
	Username  string `json:"username"`
	Available bool   `json:"available"`
}

type UpdatePrivacyRequest struct {
	LastSeen              *string `json:"lastSeen,omitempty" validate:"omitempty,oneof=everyone contacts nobody"`
	Avatar                *string `json:"avatar,omitempty" validate:"omitempty,oneof=everyone contacts nobody"`
	About                 *string `json:"about,omitempty" validate:"omitempty,oneof=everyone contacts nobody"`
	Phone                 *string `json:"phone,omitempty" validate:"omitempty,oneof=everyone contacts nobody"`
	AllowGroupAdds        *string `json:"allowGroupAdds,omitempty" validate:"omitempty,oneof=everyone contacts nobody"`
	ReadReceipts          *bool   `json:"readReceipts,omitempty"`
	DiscoverableByPhone   *bool   `json:"discoverableByPhone,omitempty"`
	ShareTypingIndicators *bool   `json:"shareTypingIndicators,omitempty"`
	SecurityNotifications *bool   `json:"securityNotifications,omitempty"`
}

type DeleteAccountRequest struct {
	Confirmation string `json:"confirmation" validate:"required,eq=DELETE"`
	RefreshToken string `json:"refreshToken" validate:"required,min=32,max=512"`
}
