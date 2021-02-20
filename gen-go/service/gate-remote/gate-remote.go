// Autogenerated by Thrift Compiler (0.13.0)
// DO NOT EDIT UNLESS YOU ARE SURE THAT YOU KNOW WHAT YOU ARE DOING

package main

import (
	"context"
	"flag"
	"fmt"
	"math"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"github.com/apache/thrift/lib/go/thrift"
	"service"
)

var _ = service.GoUnusedProtection__

func Usage() {
  fmt.Fprintln(os.Stderr, "Usage of ", os.Args[0], " [-h host:port] [-u url] [-f[ramed]] function [arg1 [arg2...]]:")
  flag.PrintDefaults()
  fmt.Fprintln(os.Stderr, "\nFunctions:")
  fmt.Fprintln(os.Stderr, "  void set_context(string conn_id, string key, string value)")
  fmt.Fprintln(os.Stderr, "  void unset_context(string conn_id, string key, string value)")
  fmt.Fprintln(os.Stderr, "  void remove_conn(string conn_id)")
  fmt.Fprintln(os.Stderr, "  void send_text(string conn_id, string message)")
  fmt.Fprintln(os.Stderr, "  void send_binary(string conn_id, string message)")
  fmt.Fprintln(os.Stderr, "  void join_group(string conn_id, string group)")
  fmt.Fprintln(os.Stderr, "  void leave_group(string conn_id, string group)")
  fmt.Fprintln(os.Stderr, "  void broadcast_binary(string group,  exclude, string message)")
  fmt.Fprintln(os.Stderr, "  void broadcast_text(string group,  exclude, string message)")
  fmt.Fprintln(os.Stderr)
  os.Exit(0)
}

type httpHeaders map[string]string

func (h httpHeaders) String() string {
  var m map[string]string = h
  return fmt.Sprintf("%s", m)
}

func (h httpHeaders) Set(value string) error {
  parts := strings.Split(value, ": ")
  if len(parts) != 2 {
    return fmt.Errorf("header should be of format 'Key: Value'")
  }
  h[parts[0]] = parts[1]
  return nil
}

func main() {
  flag.Usage = Usage
  var host string
  var port int
  var protocol string
  var urlString string
  var framed bool
  var useHttp bool
  headers := make(httpHeaders)
  var parsedUrl *url.URL
  var trans thrift.TTransport
  _ = strconv.Atoi
  _ = math.Abs
  flag.Usage = Usage
  flag.StringVar(&host, "h", "localhost", "Specify host and port")
  flag.IntVar(&port, "p", 9090, "Specify port")
  flag.StringVar(&protocol, "P", "binary", "Specify the protocol (binary, compact, simplejson, json)")
  flag.StringVar(&urlString, "u", "", "Specify the url")
  flag.BoolVar(&framed, "framed", false, "Use framed transport")
  flag.BoolVar(&useHttp, "http", false, "Use http")
  flag.Var(headers, "H", "Headers to set on the http(s) request (e.g. -H \"Key: Value\")")
  flag.Parse()
  
  if len(urlString) > 0 {
    var err error
    parsedUrl, err = url.Parse(urlString)
    if err != nil {
      fmt.Fprintln(os.Stderr, "Error parsing URL: ", err)
      flag.Usage()
    }
    host = parsedUrl.Host
    useHttp = len(parsedUrl.Scheme) <= 0 || parsedUrl.Scheme == "http" || parsedUrl.Scheme == "https"
  } else if useHttp {
    _, err := url.Parse(fmt.Sprint("http://", host, ":", port))
    if err != nil {
      fmt.Fprintln(os.Stderr, "Error parsing URL: ", err)
      flag.Usage()
    }
  }
  
  cmd := flag.Arg(0)
  var err error
  if useHttp {
    trans, err = thrift.NewTHttpClient(parsedUrl.String())
    if len(headers) > 0 {
      httptrans := trans.(*thrift.THttpClient)
      for key, value := range headers {
        httptrans.SetHeader(key, value)
      }
    }
  } else {
    portStr := fmt.Sprint(port)
    if strings.Contains(host, ":") {
           host, portStr, err = net.SplitHostPort(host)
           if err != nil {
                   fmt.Fprintln(os.Stderr, "error with host:", err)
                   os.Exit(1)
           }
    }
    trans, err = thrift.NewTSocket(net.JoinHostPort(host, portStr))
    if err != nil {
      fmt.Fprintln(os.Stderr, "error resolving address:", err)
      os.Exit(1)
    }
    if framed {
      trans = thrift.NewTFramedTransport(trans)
    }
  }
  if err != nil {
    fmt.Fprintln(os.Stderr, "Error creating transport", err)
    os.Exit(1)
  }
  defer trans.Close()
  var protocolFactory thrift.TProtocolFactory
  switch protocol {
  case "compact":
    protocolFactory = thrift.NewTCompactProtocolFactory()
    break
  case "simplejson":
    protocolFactory = thrift.NewTSimpleJSONProtocolFactory()
    break
  case "json":
    protocolFactory = thrift.NewTJSONProtocolFactory()
    break
  case "binary", "":
    protocolFactory = thrift.NewTBinaryProtocolFactoryDefault()
    break
  default:
    fmt.Fprintln(os.Stderr, "Invalid protocol specified: ", protocol)
    Usage()
    os.Exit(1)
  }
  iprot := protocolFactory.GetProtocol(trans)
  oprot := protocolFactory.GetProtocol(trans)
  client := service.NewGateClient(thrift.NewTStandardClient(iprot, oprot))
  if err := trans.Open(); err != nil {
    fmt.Fprintln(os.Stderr, "Error opening socket to ", host, ":", port, " ", err)
    os.Exit(1)
  }
  
  switch cmd {
  case "set_context":
    if flag.NArg() - 1 != 3 {
      fmt.Fprintln(os.Stderr, "SetContext requires 3 args")
      flag.Usage()
    }
    argvalue0 := flag.Arg(1)
    value0 := argvalue0
    argvalue1 := flag.Arg(2)
    value1 := argvalue1
    argvalue2 := flag.Arg(3)
    value2 := argvalue2
    fmt.Print(client.SetContext(context.Background(), value0, value1, value2))
    fmt.Print("\n")
    break
  case "unset_context":
    if flag.NArg() - 1 != 3 {
      fmt.Fprintln(os.Stderr, "UnsetContext requires 3 args")
      flag.Usage()
    }
    argvalue0 := flag.Arg(1)
    value0 := argvalue0
    argvalue1 := flag.Arg(2)
    value1 := argvalue1
    argvalue2 := flag.Arg(3)
    value2 := argvalue2
    fmt.Print(client.UnsetContext(context.Background(), value0, value1, value2))
    fmt.Print("\n")
    break
  case "remove_conn":
    if flag.NArg() - 1 != 1 {
      fmt.Fprintln(os.Stderr, "RemoveConn requires 1 args")
      flag.Usage()
    }
    argvalue0 := flag.Arg(1)
    value0 := argvalue0
    fmt.Print(client.RemoveConn(context.Background(), value0))
    fmt.Print("\n")
    break
  case "send_text":
    if flag.NArg() - 1 != 2 {
      fmt.Fprintln(os.Stderr, "SendText requires 2 args")
      flag.Usage()
    }
    argvalue0 := flag.Arg(1)
    value0 := argvalue0
    argvalue1 := flag.Arg(2)
    value1 := argvalue1
    fmt.Print(client.SendText(context.Background(), value0, value1))
    fmt.Print("\n")
    break
  case "send_binary":
    if flag.NArg() - 1 != 2 {
      fmt.Fprintln(os.Stderr, "SendBinary requires 2 args")
      flag.Usage()
    }
    argvalue0 := flag.Arg(1)
    value0 := argvalue0
    argvalue1 := []byte(flag.Arg(2))
    value1 := argvalue1
    fmt.Print(client.SendBinary(context.Background(), value0, value1))
    fmt.Print("\n")
    break
  case "join_group":
    if flag.NArg() - 1 != 2 {
      fmt.Fprintln(os.Stderr, "JoinGroup requires 2 args")
      flag.Usage()
    }
    argvalue0 := flag.Arg(1)
    value0 := argvalue0
    argvalue1 := flag.Arg(2)
    value1 := argvalue1
    fmt.Print(client.JoinGroup(context.Background(), value0, value1))
    fmt.Print("\n")
    break
  case "leave_group":
    if flag.NArg() - 1 != 2 {
      fmt.Fprintln(os.Stderr, "LeaveGroup requires 2 args")
      flag.Usage()
    }
    argvalue0 := flag.Arg(1)
    value0 := argvalue0
    argvalue1 := flag.Arg(2)
    value1 := argvalue1
    fmt.Print(client.LeaveGroup(context.Background(), value0, value1))
    fmt.Print("\n")
    break
  case "broadcast_binary":
    if flag.NArg() - 1 != 3 {
      fmt.Fprintln(os.Stderr, "BroadcastBinary requires 3 args")
      flag.Usage()
    }
    argvalue0 := flag.Arg(1)
    value0 := argvalue0
    arg53 := flag.Arg(2)
    mbTrans54 := thrift.NewTMemoryBufferLen(len(arg53))
    defer mbTrans54.Close()
    _, err55 := mbTrans54.WriteString(arg53)
    if err55 != nil { 
      Usage()
      return
    }
    factory56 := thrift.NewTJSONProtocolFactory()
    jsProt57 := factory56.GetProtocol(mbTrans54)
    containerStruct1 := service.NewGateBroadcastBinaryArgs()
    err58 := containerStruct1.ReadField2(jsProt57)
    if err58 != nil {
      Usage()
      return
    }
    argvalue1 := containerStruct1.Exclude
    value1 := argvalue1
    argvalue2 := []byte(flag.Arg(3))
    value2 := argvalue2
    fmt.Print(client.BroadcastBinary(context.Background(), value0, value1, value2))
    fmt.Print("\n")
    break
  case "broadcast_text":
    if flag.NArg() - 1 != 3 {
      fmt.Fprintln(os.Stderr, "BroadcastText requires 3 args")
      flag.Usage()
    }
    argvalue0 := flag.Arg(1)
    value0 := argvalue0
    arg61 := flag.Arg(2)
    mbTrans62 := thrift.NewTMemoryBufferLen(len(arg61))
    defer mbTrans62.Close()
    _, err63 := mbTrans62.WriteString(arg61)
    if err63 != nil { 
      Usage()
      return
    }
    factory64 := thrift.NewTJSONProtocolFactory()
    jsProt65 := factory64.GetProtocol(mbTrans62)
    containerStruct1 := service.NewGateBroadcastTextArgs()
    err66 := containerStruct1.ReadField2(jsProt65)
    if err66 != nil {
      Usage()
      return
    }
    argvalue1 := containerStruct1.Exclude
    value1 := argvalue1
    argvalue2 := flag.Arg(3)
    value2 := argvalue2
    fmt.Print(client.BroadcastText(context.Background(), value0, value1, value2))
    fmt.Print("\n")
    break
  case "":
    Usage()
    break
  default:
    fmt.Fprintln(os.Stderr, "Invalid function ", cmd)
  }
}
