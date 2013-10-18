package logstreaminput

import (
	"code.google.com/p/gomock/gomock"
	gs "github.com/rafrombrc/gospec/src/gospec"
	"os"
	"path/filepath"
	"regexp"
)

func FilehandlingSpec(c gs.Context) {
	t := &ts.SimpleT{}
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	c.Specify("The directory scanner scans a directory properly", func() {
		here, err := os.Getwd()
		c.Assume(err, gs.IsNil)
		dirPath := filepath.Join(here, "testdir")
		matchRegex, _ := regexp.MustCompile(dirPath + "/subdir1/*\\.log(\\.*)?")
		results := ScanDirectoryForLogfiles(dirPath, matchRegex)
	})
}
