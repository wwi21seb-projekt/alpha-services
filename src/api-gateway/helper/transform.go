package helper

import (
	"github.com/wwi21seb-projekt/alpha-services/src/api-gateway/schema"
	pbCommon "github.com/wwi21seb-projekt/alpha-shared/proto/common"
	pbPost "github.com/wwi21seb-projekt/alpha-shared/proto/post"
	pbUser "github.com/wwi21seb-projekt/alpha-shared/proto/user"
)

func TransformLocation(location *pbPost.Location) *schema.Location {
	if location == nil {
		return nil
	}

	return &schema.Location{
		Latitude:  *location.Latitude,
		Longitude: *location.Longitude,
		Accuracy:  *location.Accuracy,
	}
}

func LocationToProto(location *schema.Location) *pbPost.Location {
	if location == nil {
		return nil
	}

	return &pbPost.Location{
		Latitude:  &location.Latitude,
		Longitude: &location.Longitude,
		Accuracy:  &location.Accuracy,
	}
}

func TransformUser(user *pbUser.User) *schema.Author {
	if user == nil {
		return nil
	}

	return &schema.Author{
		Username: user.Username,
		Nickname: user.Nickname,
		Picture: &schema.Picture{
			Url:    user.Picture.Url,
			Width:  user.Picture.Width,
			Height: user.Picture.Height,
		},
	}
}

func AuthorToProto(author *schema.Author) *pbUser.User {
	if author == nil {
		return nil
	}

	return &pbUser.User{
		Username: author.Username,
		Nickname: author.Nickname,
		Picture: &pbCommon.Picture{
			Url:    author.Picture.Url,
			Width:  author.Picture.Width,
			Height: author.Picture.Height,
		},
	}
}

func TransformUserSubscription(subscription *pbUser.Subscription) *schema.UserSubscription {
	if subscription == nil {
		return nil
	}

	return &schema.UserSubscription{
		FollowerId:  subscription.FollowerSubscriptionId,
		FollowingId: subscription.FollowedSubscriptionId,
		Username:    subscription.Username,
		Nickname:    subscription.Nickname,
		Picture: &schema.Picture{
			Url:    subscription.Picture.Url,
			Width:  subscription.Picture.Width,
			Height: subscription.Picture.Height,
		},
	}
}
