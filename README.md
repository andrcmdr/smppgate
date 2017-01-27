# smppgate
SMS via SMPP send daemon with auto-reply and own database

### Example setup:
* Create mysql database, user and privileges on your server
* Copy config.example.json to config.json and modify it:
```
{
    // File for logging
    "logFile": "/var/log/smppgate.log",
    // SMPP gateways list
    "connectURI": [
        "smpp://login:password@127.0.0.1:2775?SourceAddrTON=5&SourceAddrNPI=1&DestAddrTON=1&DestAddrNPI=1",
        "smpp://login:password@127.0.0.2:2775"
    ],
    // Set this flag to disable send operations to SMPP gateways
    "sendDisabled": "false",
    // Mysql configuration
    "mysql": "smppgate:smppgate@tcp(localhost:3306)/smppgate?charset=utf8&parseTime=True&loc=Local",
    // Listen address, port and project path (url of project in this example - http://127.0.0.1:8881/smppgate)
    "listen": "127.0.0.1:8881",
    "projectPath": "/smppgate",
    // Check HTTP header "X-Forward-Secret" for this value (need for secure access to project if front http(s) proxy server exists in configuration)
    "forwardSecret": ""
}
```

### Usage send SMS via this gateway:

To send message do POST application/json content to service /queueSend:
```
{ from: "SENDER PHONE OR NAME",
  phone: "RECEIVER PHONE",
  text: "MESSAGE TEXT"
}
```
Example:
```
curl -H "Content-Type: application/json" -X POST -d '{"from":"ORGANIZATION","phone":"+79999999999","text":"your message"}' http://127.0.0.1:8881/smppgate/queueSend
```

To get report on 27.01.2017:
```
curl --fail --silent http://127.0.0.1:8881/smppgate/dayReport?date=2017-01-27
```
