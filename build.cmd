@echo off
@title Golang三方平台交叉编译
chcp 65001
if not exist ./build/win (
  md ./build/win
)
set /p appName=请输入软件输出名称:

set winPath=./build/win/
set macPath=./build/mac/
set liunxPath=./build/liunx/

go build -ldflags="-s -w" -x -o %winPath%%appName%.exe

SET CGO_ENABLED=0
SET GOOS=darwin
SET GOARCH=amd64

go build -ldflags="-s -w" -x -o %macPath%%appName%.app

SET CGO_ENABLED=0
SET GOOS=linux
SET GOARCH=amd64
go build -ldflags="-s -w" -x -o %liunxPath%%appName%


echo 编译完成,%winPath%%appName%.exe
echo 编译完成,%macPath%%appName%.app
echo 编译完成,%liunxPath%%appName%

pause