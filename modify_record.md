
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