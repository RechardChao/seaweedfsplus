package command

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"strings"

	"github.com/chrislusf/seaweedfs/weed/operation"
	"github.com/chrislusf/seaweedfs/weed/util"
)

var (
	d DownloadOptions
)

type DownloadOptions struct {
	server *string
	dir    *string
}

func init() {
	cmdDownload.Run = runDownload // break init cycle
	d.server = cmdDownload.Flag.String("server", "localhost:9333", "SeaweedFS master location")
	d.dir = cmdDownload.Flag.String("dir", ".", "Download the whole folder recursively if specified.")
}

var cmdDownload = &Command{
	UsageLine: "download -server=localhost:9333 -dir=one_directory fid1 [fid2 fid3 ...]",
	Short:     "download files by file id",
	Long: `download files by file id.

  Usually you just need to use curl to lookup the file's volume server, and then download them directly.
  This download tool combine the two steps into one.

  What's more, if you use "weed upload -maxMB=..." option to upload a big file divided into chunks, you can
  use this tool to download the chunks and merge them automatically.

  `,
}

func runDownload(cmd *Command, args []string) bool {
	for _, fid := range args {
		if e := downloadToFile(*d.server, fid, *d.dir); e != nil {
			fmt.Println("Download Error: ", fid, e)
		}
	}
	return true
}

func downloadToFile(server, fileId, saveDir string) error {
	fileUrl, lookupError := operation.LookupFileId(server, fileId)
	if lookupError != nil {
		return lookupError
	}
	filename, rc, err := util.DownloadUrl(fileUrl)
	if err != nil {
		return err
	}
	defer rc.Close()
	if filename == "" {
		filename = fileId
	}
	isFileList := false
	if strings.HasSuffix(filename, "-list") {
		// old command compatible
		isFileList = true
		filename = filename[0 : len(filename)-len("-list")]
	}
	f, err := os.OpenFile(path.Join(saveDir, filename), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, os.ModePerm)
	if err != nil {
		return err
	}
	defer f.Close()
	if isFileList {
		content, err := ioutil.ReadAll(rc)
		if err != nil {
			return err
		}
		fids := strings.Split(string(content), "\n")
		for _, partId := range fids {
			var n int
			_, part, err := fetchContent(*d.server, partId)
			if err == nil {
				n, err = f.Write(part)
			}
			if err == nil && n < len(part) {
				err = io.ErrShortWrite
			}
			if err != nil {
				return err
			}
		}
	} else {
		if _, err = io.Copy(f, rc); err != nil {
			return err
		}

	}
	return nil
}

func fetchContent(server string, fileId string) (filename string, content []byte, e error) {
	fileUrl, lookupError := operation.LookupFileId(server, fileId)
	if lookupError != nil {
		return "", nil, lookupError
	}
	var rc io.ReadCloser
	if filename, rc, e = util.DownloadUrl(fileUrl); e != nil {
		return "", nil, e
	}
	content, e = ioutil.ReadAll(rc)
	rc.Close()
	return
}

func WriteFile(filename string, data []byte, perm os.FileMode) error {
	f, err := os.OpenFile(filename, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	n, err := f.Write(data)
	f.Close()
	if err == nil && n < len(data) {
		err = io.ErrShortWrite
	}
	return err
}
