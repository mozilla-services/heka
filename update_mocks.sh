# We use one spelling of the mockgen command for mocks of interfaces in our own
# packages...

# pipeline.PluginHelper
$GOPATH/bin/mockgen -package=pipeline \
                    -destination=pipeline/mock_pluginhelper_test.go \
                    -self_package=github.com/mozilla-services/heka/pipeline \
                    github.com/mozilla-services/heka/pipeline PluginHelper

# pipeline.PluginRunner
$GOPATH/bin/mockgen -package=pipeline \
                    -destination=pipeline/mock_pluginrunner_test.go \
                    -self_package=github.com/mozilla-services/heka/pipeline \
                    github.com/mozilla-services/heka/pipeline PluginRunner

# pipeline.Decoder
$GOPATH/bin/mockgen -package=pipeline \
                    -destination=pipeline/mock_decoder_test.go \
                    -self_package=github.com/mozilla-services/heka/pipeline \
                    github.com/mozilla-services/heka/pipeline Decoder

# pipeline.DecoderSet
$GOPATH/bin/mockgen -package=pipeline \
                    -destination=pipeline/mock_decoderset_test.go \
                    -self_package=github.com/mozilla-services/heka/pipeline \
                    github.com/mozilla-services/heka/pipeline DecoderSet

# pipeline.DecoderRunner
$GOPATH/bin/mockgen -package=pipeline \
                    -destination=pipeline/mock_decoderrunner_test.go \
                    -self_package=github.com/mozilla-services/heka/pipeline \
                    github.com/mozilla-services/heka/pipeline DecoderRunner

# pipeline.InputRunner
$GOPATH/bin/mockgen -package=pipeline \
                    -destination=pipeline/mock_inputrunner_test.go \
                    -self_package=github.com/mozilla-services/heka/pipeline \
                    github.com/mozilla-services/heka/pipeline InputRunner

# pipeline.FilterRunner
$GOPATH/bin/mockgen -package=pipeline \
                    -destination=pipeline/mock_filterrunner_test.go \
                    -self_package=github.com/mozilla-services/heka/pipeline \
                    github.com/mozilla-services/heka/pipeline FilterRunner

# pipeline.OutputRunner
$GOPATH/bin/mockgen -package=pipeline \
                    -destination=pipeline/mock_outputrunner_test.go \
                    -self_package=github.com/mozilla-services/heka/pipeline \
                    github.com/mozilla-services/heka/pipeline OutputRunner

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

# pipeline.AMQPConnection
$GOPATH/bin/mockgen -package=pipeline \
                    -destination=pipeline/mock_amqpconnection_test.go \
                    -self_package=github.com/mozilla-services/heka/pipeline \
                    github.com/mozilla-services/heka/pipeline AMQPConnection

# pipeline.AMQPChannel
$GOPATH/bin/mockgen -package=pipeline \
                    -destination=pipeline/mock_amqpchannel_test.go \
                    -self_package=github.com/mozilla-services/heka/pipeline \
                    github.com/mozilla-services/heka/pipeline AMQPChannel

# pipeline.AMQPConnectionHub
$GOPATH/bin/mockgen -package=pipeline \
                    -destination=pipeline/mock_amqpconnectionhub_test.go \
                    -self_package=github.com/mozilla-services/heka/pipeline \
                    github.com/mozilla-services/heka/pipeline AMQPConnectionHub

# pipeline.StatAccumulator
$GOPATH/bin/mockgen -package=pipeline \
                    -destination=pipeline/mock_stataccumulator_test.go \
                    -self_package=github.com/mozilla-services/heka/pipeline \
                    github.com/mozilla-services/heka/pipeline StatAccumulator

# ...and a second spelling for mocks of interfaces that are from external packages.

# amqp.Acknowledger
$GOPATH/bin/mockgen -package=testsupport \
                    -destination=testsupport/mock_amqp_acknowledger.go \
                    github.com/streadway/amqp Acknowledger

# net.Conn
$GOPATH/bin/mockgen -package=testsupport \
                    -destination=testsupport/mock_net_conn.go \
                    net Conn

# net.Listener
$GOPATH/bin/mockgen -package=testsupport \
                    -destination=testsupport/mock_net_listener.go \
                    net Listener

# net.Error
$GOPATH/bin/mockgen -package=testsupport \
                    -destination=testsupport/mock_net_error.go \
                    net Error
