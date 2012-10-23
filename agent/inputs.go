package agent

import (
	"encoding/json"
	"github.com/howeyc/fsnotify"
	"heka/grater"
	"os"
	"time"
)

type LogfileInput struct {
	filename string
	file     *os.File
	deadline *time.Time
}

func (self *LogfileInput) LoadConfig(config *InputConfig) (err error) {
	self.filename = config.file

}

func LoadInput(config *InputConfig) (*Streamer, error) {
	switch (*config).Type {
	case "logfile":
		var plugin = new(LogfileInput)
		err := plugin.LoadConfig(config)
		return plugin, err
	}
}
