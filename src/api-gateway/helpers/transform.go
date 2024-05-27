package helpers

import (
	"github.com/wwi21seb-projekt/alpha-services/src/api-gateway/schema"
	pbPost "github.com/wwi21seb-projekt/alpha-shared/proto/post"
	pbUser "github.com/wwi21seb-projekt/alpha-shared/proto/user"
)

func TransformLocation(location *pbPost.Location) *schema.Location {
	if location == nil {
		return nil
	}

	return &schema.Location{
		Latitude:  location.Latitude,
		Longitude: location.Longitude,
		Accuracy:  location.Accuracy,
	}
}

func LocationToProto(location *schema.Location) *pbPost.Location {
	if location == nil {
		return nil
	}

	return &pbPost.Location{
		Latitude:  location.Latitude,
		Longitude: location.Longitude,
		Accuracy:  location.Accuracy,
	}
}

func TransformUser(user *pbUser.User) *schema.Author {
	if user == nil {
		return nil
	}

	return &schema.Author{
		Username:          user.Username,
		Nickname:          user.Nickname,
		ProfilePictureUrl: user.ProfilePictureUrl,
	}
}

func AuthorToProto(author *schema.Author) *pbUser.User {
	if author == nil {
		return nil
	}

	return &pbUser.User{
		Username:          author.Username,
		Nickname:          author.Nickname,
		ProfilePictureUrl: author.ProfilePictureUrl,
	}
}
