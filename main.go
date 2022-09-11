//go:generate bash -c "mkdir -p codegen && go run github.com/deepmap/oapi-codegen/cmd/oapi-codegen@latest -package codegen api/local_storage/openapi.yaml > codegen/local_storage_api.go"

package main

import (
	_ "embed"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/IceWhaleTech/CasaOS-Common/utils/file"
	util_http "github.com/IceWhaleTech/CasaOS-Common/utils/http"
	"github.com/IceWhaleTech/CasaOS-Common/utils/logger"
	gateway_common "github.com/IceWhaleTech/CasaOS-Gateway/common"
	"github.com/IceWhaleTech/CasaOS-LocalStorage/common"
	"github.com/IceWhaleTech/CasaOS-LocalStorage/pkg/cache"
	"github.com/IceWhaleTech/CasaOS-LocalStorage/pkg/config"
	"github.com/IceWhaleTech/CasaOS-LocalStorage/pkg/sqlite"
	"github.com/IceWhaleTech/CasaOS-LocalStorage/pkg/utils/command"
	"github.com/IceWhaleTech/CasaOS-LocalStorage/route"
	"github.com/IceWhaleTech/CasaOS-LocalStorage/service"
	"github.com/IceWhaleTech/CasaOS-LocalStorage/service/model"
	"github.com/coreos/go-systemd/daemon"
	"github.com/robfig/cron"
	"go.uber.org/zap"
)

const localhost = "127.0.0.1"

var (
	//go:embed api/index.html
	_docHTML string

	//go:embed api/local_storage/openapi.yaml
	_docYAML string
)

func init() {
	configFlag := flag.String("c", "", "config address")
	dbFlag := flag.String("db", "", "db path")

	versionFlag := flag.Bool("v", false, "version")

	flag.Parse()

	if *versionFlag {
		fmt.Printf("v%s\n", common.Version)
		os.Exit(0)
	}

	config.InitSetup(*configFlag)

	logger.LogInit(config.AppInfo.LogPath, config.AppInfo.LogSaveName, config.AppInfo.LogFileExt)

	if len(*dbFlag) == 0 {
		*dbFlag = config.AppInfo.DBPath
	}

	sqliteDB := sqlite.GetDB(*dbFlag)

	service.MyService = service.NewService(sqliteDB, config.CommonInfo.RuntimePath)

	service.Cache = cache.Init()

	checkSerialDiskMount()
}

func main() {
	go route.MonitoryUSB()

	crontab := cron.New()

	err := crontab.AddFunc("0/5 * * * * *", func() {
		// TODO - @tiger - call System common method to report disk utilization.
	})
	if err != nil {
		logger.Error("crontab add func error", zap.Error(err))
	}

	listener, err := net.Listen("tcp", net.JoinHostPort(localhost, "0"))
	if err != nil {
		panic(err)
	}

	apiPaths := []string{
		"/v1/usb",
		"/v1/disks",
		"/v1/storage",
		route.V2APIPath,
		route.V2DocPath,
	}
	for _, apiPath := range apiPaths {
		err = service.MyService.Gateway().CreateRoute(&gateway_common.Route{
			Path:   apiPath,
			Target: "http://" + listener.Addr().String(),
		})

		if err != nil {
			panic(err)
		}
	}

	v1Router := route.InitV1Router()
	v2Router := route.InitV2Router()
	v2DocRouter := route.InitV2DocRouter(_docHTML, _docYAML)

	mux := &util_http.HandlerMultiplexer{
		HandlerMap: map[string]http.Handler{
			"v1":  v1Router,
			"v2":  v2Router,
			"doc": v2DocRouter,
		},
	}

	if supported, err := daemon.SdNotify(false, daemon.SdNotifyReady); err != nil {
		logger.Error("Failed to notify systemd that local storage service is ready", zap.Any("error", err))
	} else if supported {
		logger.Info("Notified systemd that local storage service is ready")
	} else {
		logger.Info("This process is not running as a systemd service.")
	}

	logger.Info("LocalStorage service is listening...", zap.Any("address", listener.Addr().String()))

	server := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	err = server.Serve(listener)
	if err != nil {
		panic(err)
	}
}

func checkSerialDiskMount() {
	// check mount point
	dbList := service.MyService.Disk().GetSerialAll()

	list := service.MyService.Disk().LSBLK(true)
	mountPoint := make(map[string]string, len(dbList))
	// remount
	for _, v := range dbList {
		mountPoint[v.UUID] = v.MountPoint
	}
	for _, v := range list {
		command.ExecEnabledSMART(v.Path)
		if v.Children != nil {
			for _, h := range v.Children {
				// if len(h.MountPoint) == 0 && len(v.Children) == 1 && h.FsType == "ext4" {
				if m, ok := mountPoint[h.UUID]; ok {
					// mount point check
					volume := m
					if !file.CheckNotExist(m) {
						for i := 0; file.CheckNotExist(volume); i++ {
							volume = m + strconv.Itoa(i+1)
						}
					}
					service.MyService.Disk().MountDisk(h.Path, volume)
					if volume != m {
						ms := model.SerialDisk{}
						ms.UUID = v.UUID
						ms.MountPoint = volume
						service.MyService.Disk().UpdateMountPoint(ms)
					}

				}
				//}
			}
		}
	}
	service.MyService.Disk().RemoveLSBLKCache()
	command.OnlyExec("source " + config.AppInfo.ShellPath + "/local-storage-helper.sh ;AutoRemoveUnuseDir")
}
