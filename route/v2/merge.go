package v2

import (
	"net/http"
	"strings"

	"github.com/IceWhaleTech/CasaOS-LocalStorage/codegen"
	"github.com/IceWhaleTech/CasaOS-LocalStorage/pkg/config"
	"github.com/IceWhaleTech/CasaOS-LocalStorage/service"
	model2 "github.com/IceWhaleTech/CasaOS-LocalStorage/service/model"
	"github.com/IceWhaleTech/CasaOS-LocalStorage/service/v2/fs"

	"github.com/labstack/echo/v4"
)

var MessageMergerFSNotEnabled = "mergerfs is not enabled - either it is not enabled in configuration file; merge point is not empty before mounting; or mergerfs is not installed"

func (s *LocalStorage) GetMerges(ctx echo.Context, params codegen.GetMergesParams) error {
	if strings.ToLower(config.ServerInfo.EnableMergerFS) != "true" {
		return ctx.JSON(http.StatusServiceUnavailable, codegen.ResponseServiceUnavailable{Message: &MessageMergerFSNotEnabled})
	}

	mergesFromDB, err := service.MyService.LocalStorage().GetMergeAllFromDB(params.MountPoint)
	if err != nil {
		message := err.Error()
		return ctx.JSON(http.StatusInternalServerError, codegen.BaseResponse{Message: &message})
	}

	message := "ok"

	data := make([]codegen.Merge, 0, len(mergesFromDB))
	for _, merge := range mergesFromDB {
		// TODO - remove source volumes by UUID that are not attached, and write warnings to message
		data = append(data, MergeAdapterOut(merge))
	}

	return ctx.JSON(http.StatusOK, codegen.GetMergesResponseOK{Data: &data, Message: &message})
}

func (s *LocalStorage) SetMerge(ctx echo.Context) error {
	if strings.ToLower(config.ServerInfo.EnableMergerFS) != "true" {
		return ctx.JSON(http.StatusServiceUnavailable, codegen.ResponseServiceUnavailable{Message: &MessageMergerFSNotEnabled})
	}

	var m codegen.Merge
	if err := ctx.Bind(&m); err != nil {
		message := err.Error()
		return ctx.JSON(http.StatusBadRequest, codegen.ResponseBadRequest{Message: &message})
	}

	// default to mergerfs if fstype is not specified
	fstype := fs.MergerFSFullName
	if m.Fstype != nil {
		fstype = *m.Fstype
	}

	// expand source volume paths to source volumes
	var sourceVolumes []*model2.Volume
	if m.SourceVolumePaths != nil {
		allVolumes, err := service.MyService.Disk().GetSerialAllFromDB()
		if err != nil {
			message := err.Error()
			return ctx.JSON(http.StatusInternalServerError, codegen.BaseResponse{Message: &message})
		}

		sourceVolumes = make([]*model2.Volume, 0, len(*m.SourceVolumePaths))
		for _, volumePath := range *m.SourceVolumePaths {
			volumeFound := false
			for i := range allVolumes {
				if volumePath == allVolumes[i].Path {
					volumeFound = true
					sourceVolumes = append(sourceVolumes, &allVolumes[i])
				}
			}

			if !volumeFound {
				message := "volume " + volumePath + " not found, or it is not a CasaOS storage. Consider adding it to CasaOS first."
				return ctx.JSON(http.StatusBadRequest, codegen.ResponseBadRequest{Message: &message})
			}
		}
	}

	// set merge
	merge := &model2.Merge{
		FSType:         fstype,
		MountPoint:     m.MountPoint,
		SourceBasePath: m.SourceBasePath,
		SourceVolumes:  sourceVolumes,
	}

	merge, err := service.MyService.LocalStorage().SetMerge(merge)
	if err != nil {
		message := err.Error()
		return ctx.JSON(http.StatusInternalServerError, codegen.BaseResponse{Message: &message})
	}

	// TODO - save merge to database

	result := MergeAdapterOut(*merge)

	return ctx.JSON(http.StatusOK, codegen.SetMergeResponseOK{
		Data: &result,
	})
}

func MergeAdapterOut(m model2.Merge) codegen.Merge {
	id := int(m.ID)

	sourceVolumePaths := make([]string, 0, len(m.SourceVolumes))
	for _, volume := range m.SourceVolumes {
		sourceVolumePaths = append(sourceVolumePaths, volume.Path)
	}

	return codegen.Merge{
		Id:                &id,
		Fstype:            &m.FSType,
		MountPoint:        m.MountPoint,
		SourceBasePath:    m.SourceBasePath,
		SourceVolumePaths: &sourceVolumePaths,
		CreatedAt:         &m.CreatedAt,
		UpdatedAt:         &m.UpdatedAt,
	}
}
