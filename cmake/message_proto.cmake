if(NOT PROTOBUF_EXECUTABLE)
    message(FATAL_ERROR "Google protocol buffers 'protoc' must be installed, message.proto has been modified and needs to be regenerated.")
endif()

execute_process(
COMMAND ${PROTOBUF_EXECUTABLE} --gogo_out=. -I=.:../build/heka/src/code.google.com/p/gogoprotobuf/gogoproto:../build/heka/src/code.google.com/p/gogoprotobuf/protobuf message.proto
WORKING_DIRECTORY "${SRC_DIR}/message"
)
