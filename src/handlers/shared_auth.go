package handlers

import (
	"context"
	"errors"

	"yggdrasil-api-go/src/sharedauth"
	"yggdrasil-api-go/src/utils"
	"yggdrasil-api-go/src/yggdrasil"

	"github.com/gin-gonic/gin"
)

type sharedProfileProvider interface {
	GetSharedProfile(context.Context, sharedauth.Identity) (*yggdrasil.Profile, error)
}

func sharedProfiles(identities []sharedauth.Identity) []yggdrasil.Profile {
	profiles := make([]yggdrasil.Profile, 0, len(identities))
	for _, identity := range identities {
		profiles = append(profiles, yggdrasil.Profile{
			ID: sharedauth.FormatUUID(identity.UUID), Name: identity.Name,
			Properties: []yggdrasil.ProfileProperty{},
		})
	}
	return profiles
}

func respondSharedAuthError(c *gin.Context, err error, invalidToken bool) {
	switch {
	case errors.Is(err, sharedauth.ErrInvalid), errors.Is(err, sharedauth.ErrIdentityConflict):
		if invalidToken {
			utils.RespondInvalidToken(c)
		} else {
			utils.RespondInvalidCredentials(c)
		}
	case errors.Is(err, sharedauth.ErrNotReady), errors.Is(err, sharedauth.ErrCommitUnknown):
		utils.RespondError(c, 503, "ServiceUnavailable", "Shared authentication is unavailable")
	default:
		utils.RespondError(c, 500, "InternalServerError", "Shared authentication failed")
	}
}
