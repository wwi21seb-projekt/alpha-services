package helper

import (
	"github.com/wwi21seb-projekt/alpha-services/src/api-gateway/schema"
	userv1 "github.com/wwi21seb-projekt/alpha-shared/gen/server_alpha/user/v1"
)

func TransformUserSubscription(subscription *userv1.Subscription) *schema.UserSubscription {
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
