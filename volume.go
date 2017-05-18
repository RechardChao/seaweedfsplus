package command

import (
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/chrislusf/seaweedfs/weed/glog"
	"github.com/chrislusf/seaweedfs/weed/server"
	"github.com/chrislusf/seaweedfs/weed/storage"
	"github.com/chrislusf/seaweedfs/weed/util"
)

var (
	v VolumeServerOptions
)

type VolumeServerOptions struct {
	port                  *int
	publicPort            *int
	folders               []string
	folderMaxLimits       []int
	ip                    *string
	publicUrl             *string
	bindIp                *string
	master                *string
	pulseSeconds          *int
	idleConnectionTimeout *int
	maxCpu                *int
	dataCenter            *string
	rack                  *string
	whiteList             []string
	indexType             *string
	fixJpgOrientation     *bool
	readRedirect          *bool
	enableBytesCache      *bool
}

func init() {
	cmdVolume.Run = runVolume // break init cycle
	v.port = cmdVolume.Flag.Int("port", 8080, "http listen port")
	v.publicPort = cmdVolume.Flag.Int("port.public", 0, "port opened to public")
	v.ip = cmdVolume.Flag.String("ip", "", "ip or server name")
	v.publicUrl = cmdVolume.Flag.String("publicUrl", "", "Publicly accessible address")
	v.bindIp = cmdVolume.Flag.String("ip.bind", "0.0.0.0", "ip address to bind to")
	v.master = cmdVolume.Flag.String("mserver", "localhost:9333", "master server location")
	v.pulseSeconds = cmdVolume.Flag.Int("pulseSeconds", 5, "number of seconds between heartbeats, must be smaller than or equal to the master's setting")
	v.idleConnectionTimeout = cmdVolume.Flag.Int("idleTimeout", 10, "connection idle seconds")
	v.maxCpu = cmdVolume.Flag.Int("maxCpu", 0, "maximum number of CPUs. 0 means all available CPUs")
	v.dataCenter = cmdVolume.Flag.String("dataCenter", "", "current volume server's data center name")
	v.rack = cmdVolume.Flag.String("rack", "", "current volume server's rack name")
	v.indexType = cmdVolume.Flag.String("index", "memory", "Choose [memory|leveldb|boltdb] mode for memory~performance balance.")
	v.fixJpgOrientation = cmdVolume.Flag.Bool("images.fix.orientation", true, "Adjust jpg orientation when uploading.")
	v.readRedirect = cmdVolume.Flag.Bool("read.redirect", true, "Redirect moved or non-local volumes.")
	v.enableBytesCache = cmdVolume.Flag.Bool("cache.enable", false, "direct cache instead of OS cache, cost more memory.")
}

var cmdVolume = &Command{
	UsageLine: "volume -port=8080 -dir=/tmp -max=5 -ip=server_name -mserver=localhost:9333",
	Short:     "start a volume server",
	Long: `start a volume server to provide storage spaces

  `,
}

var (
	volumeFolders         = cmdVolume.Flag.String("dir", os.TempDir(), "directories to store data files. dir[,dir]...")
	maxVolumeCounts       = cmdVolume.Flag.String("max", "7", "maximum numbers of volumes, count[,count]...")
	volumeWhiteListOption = cmdVolume.Flag.String("whiteList", "", "comma separated Ip addresses having write permission. No limit if empty.")
)

func runVolume(cmd *Command, args []string) bool {
	if *v.maxCpu < 1 {
		*v.maxCpu = runtime.NumCPU()
	}
	runtime.GOMAXPROCS(*v.maxCpu)

	//Set multiple folders and each folder's max volume count limit'
	v.folders = strings.Split(*volumeFolders, ",")
	maxCountStrings := strings.Split(*maxVolumeCounts, ",")
	for _, maxString := range maxCountStrings {
		if max, e := strconv.Atoi(maxString); e == nil {
			v.folderMaxLimits = append(v.folderMaxLimits, max)
		} else {
			glog.Fatalf("The max specified in -max not a valid number %s", maxString)
		}
	}
	if len(v.folders) != len(v.folderMaxLimits) {
		glog.Fatalf("%d directories by -dir, but only %d max is set by -max", len(v.folders), len(v.folderMaxLimits))
	}
	for _, folder := range v.folders {
		if err := util.TestFolderWritable(folder); err != nil {
			glog.Fatalf("Check Data Folder(-dir) Writable %s : %s", folder, err)
		}
	}

	//security related white list configuration
	if *volumeWhiteListOption != "" {
		v.whiteList = strings.Split(*volumeWhiteListOption, ",")
	}

	if *v.ip == "" {
		*v.ip = "127.0.0.1"
	}

	if *v.publicPort == 0 {
		*v.publicPort = *v.port
	}
	if *v.publicUrl == "" {
		*v.publicUrl = *v.ip + ":" + strconv.Itoa(*v.publicPort)
	}
	isSeperatedPublicPort := *v.publicPort != *v.port

	volumeMux := http.NewServeMux()
	publicVolumeMux := volumeMux
	if isSeperatedPublicPort {
		publicVolumeMux = http.NewServeMux()
	}

	volumeNeedleMapKind := storage.NeedleMapInMemory
	switch *v.indexType {
	case "leveldb":
		volumeNeedleMapKind = storage.NeedleMapLevelDb
	case "boltdb":
		volumeNeedleMapKind = storage.NeedleMapBoltDb
	}
	volumeServer := weed_server.NewVolumeServer(volumeMux, publicVolumeMux,
		*v.ip, *v.port, *v.publicUrl,
		v.folders, v.folderMaxLimits,
		volumeNeedleMapKind,
		*v.master, *v.pulseSeconds, *v.dataCenter, *v.rack,
		v.whiteList,
		*v.fixJpgOrientation, *v.readRedirect,
		*v.enableBytesCache,
	)

	listeningAddress := *v.bindIp + ":" + strconv.Itoa(*v.port)
	glog.V(0).Infoln("Start Seaweed volume server", util.VERSION, "at", listeningAddress)
	listener, e := util.NewListener(listeningAddress, time.Duration(*v.idleConnectionTimeout)*time.Second)
	if e != nil {
		glog.Fatalf("Volume server listener error:%v", e)
	}
	if isSeperatedPublicPort {
		publicListeningAddress := *v.bindIp + ":" + strconv.Itoa(*v.publicPort)
		glog.V(0).Infoln("Start Seaweed volume server", util.VERSION, "public at", publicListeningAddress)
		publicListener, e := util.NewListener(publicListeningAddress, time.Duration(*v.idleConnectionTimeout)*time.Second)
		if e != nil {
			glog.Fatalf("Volume server listener error:%v", e)
		}
		go func() {
			if e := http.Serve(publicListener, publicVolumeMux); e != nil {
				glog.Fatalf("Volume server fail to serve public: %v", e)
			}
		}()
	}

	OnInterrupt(func() {
		volumeServer.Shutdown()
	})

	if e := http.Serve(listener, volumeMux); e != nil {
		glog.Fatalf("Volume server fail to serve: %v", e)
	}
	return true
}
