[![Build Status](https://travis-ci.org/kitech/toxtun.svg?branch=master)](https://travis-ci.org/kitech/toxtun)

### toxtun

基于tox安全P2P网络的TCP tunnel实现。

### 功能特性

* 连接两个位于防火墙后的网络节点
* 连接两个不能直接通信的网络节点


### 依赖包

    go get -u github.com/kitech/go-toxcore
    go get -u github.com/cyfdecyf/color
    go get -u github.com/bitly/go-simplejson
    go get -u github.com/GianlucaGuarini/go-observable
    go get -u github.com/go-ini/ini
    

### 安装

    go get -u github.com/kitech/toxtun
    
或者，

    git clone https://github.com/kitech/toxtun
    cd toxtun
    go build -v


### 启动服务端

    toxtun -kcp-mode fast server


### 启动客户端

编辑toxtun.ini配置文件，把启动server端时的toxid写入配置文件，

    [server]
    name = whttpd
    
    [client]
    toxtun1 = *:81:127.0.0.1:8181:A5A02FECA08E3EAC7E646B89C0507A8AAF9136E05DC756FF09F86230951820670F908F2E7719
    toxtun2 = *:82:127.0.0.1:8282:6BA28AC06C1D57783FE017FA9322D0B356E61404C92155A04F64F3B19C75633E8BDDEFFA4856

注：目前只能使用第一个tunnel配置项。

启动客户端：

    toxtun -config toxtun.ini -kcp-mode fast client


### TODOs
- [x] 配置参数
- [x] 统计服务模块
- [ ] 多tunnel支持
- [ ] 数据编码：JSON=>MsgPack
- [ ] toxnet/friend失联重连
- [x] 关闭连接原因
- [ ] 活动连接读写超时
- [ ] 应该还能再加快传输速度


### 创建连接流程

每个连接分布一个KCP。所以，KCP创建需要一个协商过程。

* tun客户端接收到客户端请求连接
* tun客户端创建半连接Channel对象，保存该连接。
* 通过tox FriendSendMessage发送KCP连接协商请求，需要确定消息发送成功。
* tun服务端收到KCP连接协商请求，创建Channel对象，并发送分配的KCP的conv结果。
* tun服务端创建KCP实例，与上一步创建的Channel对象关联。
* tun客户端收到协商响应，创建KCP实现，与当前的Channel对象关系。


### 关闭连接流程

* 单向发送FIN包，由于使用的可靠传输，不再回ACK包。
* 采用promise检查客户端关闭，或者服务端关闭，或者同时关闭。


### 协商KCP的conv机制
* KCP conv值的协商，根据当前时间与客户端ID实现。也就是动态conv值，防止产生碰撞冲突。
* 需要设置一个基准时间，而不是绝对时间。（也可以用绝对时间，还可以加上其他信息，像服务器IP与端口）。
* 两个值结合起来，生成crc32值，作为KCP的conv(sation)值。
* 总的来说，是客户端发起协商请求，服务端计算并分配conv(sation)值。
* 协商过程不使用KCP连接，而是使用tox FriendMessage，是因为KCP连接无法做连接超时控制。

### 数据传输速度

KCP+tox(lossy packet)默认配置：100K+/s

开启KCP NoDelay模式后：760K/s (调整kcp update interval)

测试情况说明，youtube视频连续播放测试1天，传输视频数据3G。

[详细统计数据](docs/stats.md)


### 线程模型

由于有回调的事件模式，会涉及多线程的数据结构操作，因此要考虑线程同步问题，否则出现一些程序崩溃问题。

第一种，全部使用同一线程，其他线程的数据库通过channel传递到该事件处理线程（主线程）进行处理。

要注意，已经在同一线程中的操作不需要再通过channel发送事件，

这样更直接，也更不容易导致多channel的死锁问题。

客户端goroutine个数，main+tox+kcp+client*n+check connect timeout

服务端goroutine个数，main+tox+kcp+client*n


第二种，使用锁，对产生多线程操作冲突的代码通过锁来同步。(已弃用)


### 数据传输选型

虽然可以直接用tox的lossless packet，但仍旧要加一层KCP，因为toxnet可能暂时性掉线(offline)。

如果有活动连接和数据包，则要考虑自己处理重发。


Contributing
------------
1. Fork it
2. Create your feature branch (``git checkout -b my-new-feature``)
3. Commit your changes (``git commit -am 'Add some feature'``)
4. Push to the branch (``git push origin my-new-feature``)
5. Create new Pull Request

