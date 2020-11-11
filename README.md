# fixc
simple FIX protocol client for golang

## FMZ FIX协议插件程序

- 项目目录：

  .
  ├── main.go
  ├── fixc
       ├── fix_client.go
       ├── msgtype.go


  
  main.go : 插件程序，衔接托管者程序（FMZ托管者程序robot）和交易所接口，使交易所接口通过插件程序接入到FMZ平台。
  fixc目录 : FIX协议客户端库，包含```fix_client.go```，```msgtype.go```。

- 使用
  范例```main.go```对接的是FTX交易所的FIX协议接口。

  FMZ平台机器人测试代码：
  ```
  function main() {
      exchange.SetCurrency("BTC-PERP")
      var id = exchange.Buy(8000, 0.01)
      Log(id)
      Sleep(1000 * 10)
      Log("开始撤销订单:", id)
      exchange.CancelOrder(id)
  }
  ```

  在FMZ平台配置好FIX通用协议交易所对象之后，接着部署一个托管者程序，然后可以直接运行```main.go```（和托管者程序在同一设备），运行插件程序，然后使用FMZ平台以上代码创建机器人运行，插件程序开始工作：

  ```
  2020/11/11 16:16:09 Running  http://127.0.0.1:8888/FTX ...
  Send: 8=FIX.4.2|9=154|49=xxxxxxxxxxxxxx|56=FTX|34=1|52=20201111-08:16:17.127|35=D|11=fmz1605082577127|44=8000|54=1|59=1|21=1|55=BTC-PERP|40=2|38=0.01|10=052|
  onError: dial tcp 13.114.178.121:4363: i/o timeout
  Send: 8=FIX.4.2|9=162|35=A|49=xxxxxxxxxxxxxx|56=FTX|34=1|52=20201111-08:16:51|98=0|108=30|96=1f966217ca6e85265d7ca8b176c6f19db011ba7b1b088cbb8d0ea49073f16df0|10=051|
  onError: tls: DialWithDialer timed out
  Send: 8=FIX.4.2|9=162|35=A|49=xxxxxxxxxxxxxx|56=FTX|34=1|52=20201111-08:17:29|98=0|108=30|96=70a3a0a3624c718eb9ac8a324aa6e7969697c384617d3cdec24fd6de91e7dc28|10=079|
  receive: 8=FIX.4.2|9=98|35=A|49=FTX|56=xxxxxxxxxxxxxx|34=1|52=20201111-08:17:29.628|98=0|108=30|10=133|

  ...
  ```

  


