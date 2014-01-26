
在file_output.go的func (o *FileOutput) handleMessage(pack *PipelinePack, outBytes *[]byte) (err error)中注释掉一行， 为了让输入结果不产生空行

```
	 	case "text":
		*outBytes = append(*outBytes, *pack.Message.Payload...)
		//*outBytes = append(*outBytes, NEWLINE)
```

实现功能，第一次读文件时，从尾部读取 在/heka/plugins/file/logfile_input.go函数func (fm *FileMonitor) Init(conf *LogfileInputConfig) (err error)中添加如下代码

```
    //fm.seek = 0
    var fd *os.File
    if fd, err = os.Open(file); err != nil {
       return
    }
    defer fd.Close()
    fm.seek, _ = fd.Seek(0, 2)
```

tcp_output增加一个配置参数exitonfailure = false/true，当tcp连接出错时，主动关闭hekad进程。 可以另外增加一个脚本，检测hekad进程不在时，自动重启hekad进程。参考

```
 
  // Output plugin that sends messages via TCP using the Heka protocol.
  type TcpOutput struct {
 -  address    string
 -  connection net.Conn
 +  address       string
 +  connection    net.Conn
 +  exitonfailure bool
  }
  
  // ConfigStruct for TcpOutput plugin.
  type TcpOutputConfig struct {
    // String representation of the TCP address to which this output should be
    // sending data.
 -  Address string
 +  Address       string
 +  ExitOnFailure bool
  }
  
  func (t *TcpOutput) ConfigStruct() interface{} {
 -  return &TcpOutputConfig{Address: "localhost:9125"}
 +  return &TcpOutputConfig{Address: "localhost:9125", ExitOnFailure: false}
  }
  
  func (t *TcpOutput) Init(config interface{}) (err error) {
    conf := config.(*TcpOutputConfig)
    t.address = conf.Address
 +  t.exitonfailure = conf.ExitOnFailure
    t.connection, err = net.Dial("tcp", t.address)
    return
  }
 @@ -61,6 +64,9 @@ func (t *TcpOutput) Run(or OutputRunner, h PluginHelper) (err error) {
  
      if n, e = t.connection.Write(outBytes); e != nil {
        or.LogError(fmt.Errorf("writing to %s: %s", t.address, e))
 +      if t.exitonfailure {
 +        return
 +      }
      } else if n != len(outBytes) {
        or.LogError(fmt.Errorf("truncated output to: %s", t.address))
      }
```