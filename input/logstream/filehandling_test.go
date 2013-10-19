package logstreaminput

import (
	gs "github.com/rafrombrc/gospec/src/gospec"
	"os"
	"path/filepath"
	"regexp"
)

func FilehandlingSpec(c gs.Context) {
	c.Specify("The directory scanner", func() {
		here, err := os.Getwd()
		c.Assume(err, gs.IsNil)
		dirPath := filepath.Join(here, "testdir")

		c.Specify("scans a directory properly", func() {
			matchRegex := regexp.MustCompile(dirPath + "/subdir1/*\\.log(\\.*)?")
			results := ScanDirectoryForLogfiles(dirPath, matchRegex)
			c.Expect(len(results), gs.Equals, 3)
		})

		c.Specify("scana a directory with a bad regexp", func() {
			matchRegex := regexp.MustCompile(dirPath + "/subdir1/*\\.logg(\\.*)?")
			results := ScanDirectoryForLogfiles(dirPath, matchRegex)
			c.Expect(len(results), gs.Equals, 0)
		})
	})
}
