本程序只是简单的劫持HTTP HTTPS流量进行指定转发 不应该用于生产环境

# 源码编译
    1. 安装最新版本golang程序
    2. go mod tidy   # 下载相关依赖
    3. ./build.cmd   # 执行编译脚本 输出目录默认 build 目录

# 程序运行(ubuntu)
    chomod +x ./build/liunx/main
    ./build/liunx/main

# 程序运行(windows)
    ./build/win/main.exe

# 命令提示
    -h

PS: 请开放你指定的端口

声明: 请勿用于非法用途 本程序只是一个示例代码 只能用于开发测试 不可部署在生产环境！
