

### 连接流程

每个连接分布一个KCP。所以，KCP创建需要一个协商过程。

* tun客户端接收到客户端请求连接
* tun客户端创建半连接Channel对象，保存该连接。
* 通过tox FriendSendMessage发送KCP连接协商请求，需要确定消息发送成功。
* tun服务端收到KCP连接协商请求，创建Channel对象，并发送分配的KCP的conv结果。
* tun服务端创建KCP实例，与上一步创建的Channel对象关联。
* tun客户端收到协商响应，创建KCP实现，与当前的Channel对象关系。


### 协商KCP的conv机制
* KCP conv值的协商，根据当前时间与客户端ID实现。也就是动态conv值，防止产生碰撞冲突。
* 需要设置一个基准时间，而不是绝对时间。（也可以用绝对时间，还可以加上其他信息，像服务器IP与端口）。
* 两个值结合起来，生成crc32值，作为KCP的conv(sation)值。
* 总的来说，是客户端发起协商请求，服务端计算并分配conv(sation)值。

### 速度

KCP默认配置：40-50K/s


