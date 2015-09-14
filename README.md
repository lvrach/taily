# taily
stream your logs easily over the internet


**NOTE:** This is an early beta version

Use standard unix tools to stream your logs into tailbin.
You can then monitor your service from a browser, or share them with others.
            
### Using netcat 

The following command will open a tcp connection with our server.
Our server will respond with a url that directs you to a unique page for your stream.
Everything you write, will apear in there.

`nc tailbin.com 4444` 

### Paste a file 
You could share a file from your terminal by just pipelining cat output to the previous command.
` cat foo.txt | nc tailbin.com 4444 `

### tail -f your logs
Open a log file with tail -f and pipeline it to the netcat command.
Every new line added to the file will be pushed to the server.
Also notice that the page is being updated in realtime. 
`tail -f /var/log/apache2/access.log | nc tailbin.com 4444 `

### Monitor your server 
Here some tricks you can use to monitor the health of your servers:
####Uptime & cpu load:
`(while true; do uptime;  sleep 2 ; done;) | nc tailbin.com 4444 `

####Memory usage:
` (while true; do free -h; sleep 10; done;) | nc tailbin.com 4444 `
####Hard disk usage:
` (while true; do df -h;   sleep 10; done;) | nc tailbin.com 4444 `
