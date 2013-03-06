# We use one spelling of the mockgen command for mocks of interfaces in our own
# packages...

# pipeline.MockPluginHelper
$GOPATH/bin/mockgen -package=pipeline \
                    -destination=pipeline/mock_pluginhelper_test.go \
                    -self_package=github.com/mozilla-services/heka/pipeline \
                    github.com/mozilla-services/heka/pipeline PluginHelper

# pipeline.DecoderRunner
$GOPATH/bin/mockgen -package=pipeline \
                    -destination=pipeline/mock_decoderrunner_test.go \
                    -self_package=github.com/mozilla-services/heka/pipeline \
                    github.com/mozilla-services/heka/pipeline DecoderRunner

# pipeline.Input
$GOPATH/bin/mockgen -package=pipeline \
                    -destination=pipeline/mock_input_test.go \
                    -self_package=github.com/mozilla-services/heka/pipeline \
                    github.com/mozilla-services/heka/pipeline Input


# pipeline.WhisperRunner
$GOPATH/bin/mockgen -package=pipeline \
                    -destination=pipeline/mock_whisperrunner_test.go \
                    -self_package=github.com/mozilla-services/heka/pipeline \
                    github.com/mozilla-services/heka/pipeline WhisperRunner

# ...and a second spelling for mocks of interfaces that are from external packages.

# net.Conn
$GOPATH/bin/mockgen -package=testsupport \
                    -destination=testsupport/mock_net_conn.go \
                    net Conn

# net.Listener
$GOPATH/bin/mockgen -package=testsupport \
                    -destination=testsupport/mock_net_listener.go \
                    net Listener
