set GOARCH=arm
set GOOS=linux
go build -o ..\bin\relayhat ..\cmd\main.go

rem set GOARCH=386
rem set GOOS=windows
rem go build -o ..\bin\relayhat.exe ..\cmd\main.go
