package pipeline

var AvailablePlugins = map[string]func() Plugin{
	"MessageGeneratorInput": func() Plugin { return new(MessageGeneratorInput) },
	"UdpInput":              func() Plugin { return new(UdpInput) },
	"UdpGobInput":           func() Plugin { return new(UdpGobInput) },
	"JsonDecoder":           func() Plugin { return new(JsonDecoder) },
	"GobDecoder":            func() Plugin { return new(GobDecoder) },
	"StatRollupFilter":      func() Plugin { return new(StatRollupFilter) },
	"NamedOutputFilter":     func() Plugin { return new(NamedOutputFilter) },
	"LogFilter":             func() Plugin { return new(LogFilter) },
	"LogOutput":             func() Plugin { return new(LogOutput) },
	"CounterOutput":         func() Plugin { return new(CounterOutput) },
}
