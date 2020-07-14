# Fund-Rotor
A tool that use Websock to crawl and calculate fund's estimated returns for the day

## Used way
### Add The Fund Info
需要在front-end.html中的js代码区域填写要获取的基金信息
<br>
```js
//此处填写要实时爬取的基金代码和该基金的持有份额(可配置多个)
    var config=[
        {"code":"519674","money":"10"},
        {"code":"320007","money":"46"},
    ]
```

### Start The Server
windows平台的可以直接打开项目中的可执行文件`Fund-Rotor.exe`,然后在打开前端页面`front-end.html`,点击页面上的open按钮，就可以实时监听获取基金的收益信息了(服务端代码设置了10秒推送一次)
