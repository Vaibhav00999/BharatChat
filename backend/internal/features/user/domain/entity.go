package domain

import "time"

type PrivacyLevel string

const (
	PrivacyEveryone PrivacyLevel = "everyone"
	PrivacyContacts PrivacyLevel = "contacts"
	PrivacyNobody   PrivacyLevel = "nobody"
)

type User struct {
	ID, PhoneNumber, CountryCode                 string
	Username                                     *string
	DisplayName                                  string
	AvatarURL                                    *string
	About                                        string
	IsVerified                                   bool
	LastSeenAt                                   *time.Time
	IsOnline                                     bool
	PrivacyLastSeen, PrivacyAvatar, PrivacyAbout PrivacyLevel
	PrivacyReadReceipts                          bool
	PrivacyPhone                                 PrivacyLevel
	DiscoverableByPhone                          bool
	AllowGroupAdds                               PrivacyLevel
	ShareTypingIndicators                        bool
	SecurityNotifications                        bool
	CreatedAt, UpdatedAt                         time.Time
}
type ProfileUpdate struct{ DisplayName, Username, About, AvatarURL *string }

type PrivacyUpdate struct {
	LastSeen              *PrivacyLevel
	Avatar                *PrivacyLevel
	About                 *PrivacyLevel
	Phone                 *PrivacyLevel
	AllowGroupAdds        *PrivacyLevel
	ReadReceipts          *bool
	DiscoverableByPhone   *bool
	ShareTypingIndicators *bool
	SecurityNotifications *bool
}
