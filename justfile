set quiet
set export
set dotenv-load
#set positional-arguments

#set shell := ["powershell.exe", "-c"]
set shell := ["cmd.exe", "/c"]

build:
    go build -ldflags "-s -w"
#    go build -ldflags -H=windowsgui
#    go build -ldflags "-H=windowsgui -s -w"

gen:
    go generate