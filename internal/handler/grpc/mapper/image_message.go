package mapper

import (
	"strconv"

	"github.com/google/uuid"

	impb "github.com/webitel/im-thread-service/gen/go/thread/v1"
	"github.com/webitel/im-thread-service/internal/service/dto"
)

func MapToSendImageRequest(in *impb.SendImageRequest) *dto.SendImageRequest {
	if in == nil {
		return nil
	}

	var imgReq dto.ImageRequest

	imgReq.Body = in.GetBody()
	imgReq.Images = make([]*dto.Image, 0, len(in.GetImages()))

	for _, img := range in.GetImages() {
		id, _ := strconv.ParseInt(img.GetId(), 10, 64)
		imgReq.Images = append(imgReq.Images, &dto.Image{
			ID:       id,
			URL:      img.GetLink(),
			MimeType: img.GetMimeType(),
			Name:     img.GetName(),
		})
	}

	sendAs, _ := uuid.Parse(in.GetSendAs())

	return &dto.SendImageRequest{
		From:     MapPeerFromProto(in.GetFrom()),
		To:       MapPeerFromProto(in.GetTo()),
		Image:    imgReq,
		DomainID: in.GetDomainId(),
		SendID:   in.GetSendId(),
		SendAs:   &sendAs,
	}
}

func MapToSendImageResponse(out *dto.SendImageResponse) *impb.SendImageResponse {
	if out == nil {
		return nil
	}

	return &impb.SendImageResponse{
		Id: out.ID.String(),
		To: MapPeerToProto(out.To),
	}
}
