
### 客户端统计数据

    toxtun stats: client
    Online Status: self: true/4, peer: true/26
    Connections: total: 3742, ok: 3569, err: 173, active: 3, chans: 3, real1: 3, real2: 3
    Close Reasons: map[server\_close:2326 server\_close,both\_close:2326 connect\_timeout:151 both\_close:1 client\_close:1239]
    DataStream: req: 10192798, resp: 1863592824, r/s: 1/183, reqnet: 16906507, respnet: 2491620531, cost: 1.34
    goroutines: cur: 12, max: 88, min: 4
    memory: cur: 3400264, max: 6319608, min: 324144, total: 38373356736


### 服务端统计数据

    toxtun stats: server
    Online Status: self: true/5, peer: true/70
    Connections: total: 5950, ok: 5950, err: 0, active: 29, chans: 27, real1: 29, real2: 29
    DataStream: recv: 16694728, send: 2296697131, r/s: 1/138
    goroutines: cur: 38, max: 69, min: 8
    memory: cur: 65712744, max: 103190448, min: 318184, total: 47132792696
    
### 网络状态

lantern:

    [download]   5.4% of 285.44MiB at 40.66KiB/s ETA 01:53:17

toxtun:

    [download]  11.7% of 285.44MiB at 28.19KiB/s ETA 02:32:39
    

还有点差距。

