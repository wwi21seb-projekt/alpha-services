package helpers

import (
	"github.com/wwi21seb-projekt/alpha-services/src/api-gateway/schema"
	pb "github.com/wwi21seb-projekt/alpha-shared/proto/post"
)

func TransformLocation(location *pb.Location) *schema.Location {
	if location == nil {
		return nil
	}

	return &schema.Location{
		Latitude:  location.Latitude,
		Longitude: location.Longitude,
		Accuracy:  location.Accuracy,
	}
}

func LocationToProto(location *schema.Location) *pb.Location {
	if location == nil {
		return nil
	}

	return &pb.Location{
		Latitude:  location.Latitude,
		Longitude: location.Longitude,
		Accuracy:  location.Accuracy,
	}
}

func TransformAuthor(author *pb.Profile) *schema.Author {
	if author == nil {
		return nil
	}

	return &schema.Author{
		Username:          author.Username,
		Nickname:          author.Nickname,
		ProfilePictureUrl: author.ProfilePictureUrl,
	}
}

func AuthorToProto(author *schema.Author) *pb.Profile {
	if author == nil {
		return nil
	}

	return &pb.Profile{
		Username:          author.Username,
		Nickname:          author.Nickname,
		ProfilePictureUrl: author.ProfilePictureUrl,
	}
}
