package mapper

import (
	"strconv"

	impb "github.com/webitel/im-thread-service/gen/go/thread/v1"
	"github.com/webitel/im-thread-service/internal/service/dto"
)

func MapToSendImageRequest(in *impb.SendImageRequest) *dto.SendImageRequest {
	if in == nil {
		return nil
	}

	var imgReq dto.ImageRequest
	if pbImg := in.GetImage(); pbImg != nil {
		imgReq.Body = pbImg.GetBody()
		imgReq.Images = make([]*dto.Image, 0, len(pbImg.GetImages()))

		for _, img := range pbImg.GetImages() {
			id, _ := strconv.ParseInt(img.GetId(), 10, 64)
			imgReq.Images = append(imgReq.Images, &dto.Image{
				ID:       id,
				URL:      img.GetLink(),
				MimeType: img.GetMimeType(),
				Name:     img.GetName(),
			})
		}
	}

	return &dto.SendImageRequest{
		From:     MapPeerFromProto(in.GetFrom()),
		To:       MapPeerFromProto(in.GetTo()),
		Image:    imgReq,
		DomainID: in.GetDomainId(),
		SendID:   in.GetSendId(),
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
