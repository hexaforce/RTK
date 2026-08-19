# RTKサービス利用時の NTRIP 通信まとめ

## 前提

docomo、SoftBank
などの商用RTKサービスを利用する場合、利用者側では以下を基本的に意識しません。

-   基準局の構成
-   上流の補正情報生成処理
-   どの基準局が選択されているか
-   移動時の基準局ハンドオーバー処理

これらは **RTKサービス事業者側の内部処理（ブラックボックス）**
として扱います。

利用者側で意識する主な登場人物は次の3つです。

1.  GNSS Receiver
2.  NTRIP Client（車載PCなど）
3.  RTK Provider（docomo / SoftBank / KDDI など）

------------------------------------------------------------------------

## 全体構成

``` mermaid
flowchart LR
    GNSS["GNSS Receiver<br/>RTK対応GNSS受信機"]
    CLIENT["NTRIP Client<br/>車載PC / Raspberry Pi / Jetson等"]
    PROVIDER["RTK Provider<br/>docomo / SoftBank / KDDI等"]

    GNSS -->|"NMEA-GGA"| CLIENT
    CLIENT -->|"GGA / NTRIP通信"| PROVIDER
    PROVIDER -->|"RTCM補正情報"| CLIENT
    CLIENT -->|"RTCM"| GNSS
```

GNSS Receiver が現在位置を含む **NMEA-GGA** を出力し、NTRIP Client
がRTKサービスへ送信します。

RTKサービスから返ってきた **RTCM補正情報** をNTRIP ClientがGNSS
Receiverへ投入することで、GNSS Receiver側でRTK測位を行います。

------------------------------------------------------------------------

## 通信シーケンス

``` mermaid
sequenceDiagram
    participant G as GNSS Receiver
    participant C as NTRIP Client
    participant P as RTK Provider

    G->>C: NMEA-GGA

    C->>P: TCP/TLS接続
    C->>P: NTRIP GET / 認証<br/>Mountpoint・ID・Password
    P-->>C: 接続成功

    loop 測位中
        G->>C: NMEA-GGA
        C->>P: GGA（現在位置）
        P-->>C: RTCM補正情報
        C->>G: RTCM
        Note over G: RTK測位<br/>FLOAT → FIX
    end
```

------------------------------------------------------------------------

## NTRIP Clientから見た処理

``` mermaid
flowchart TD
    START["NTRIP Client 起動"]
    CONFIG["Provider設定読込<br/>Host / Port / Mountpoint<br/>Username / Password"]
    CONNECT["RTK Providerへ接続"]
    AUTH["NTRIP認証"]
    GGA["GNSS Receiverから<br/>GGA取得"]
    SEND["ProviderへGGA送信"]
    RECEIVE["RTCM受信"]
    OUTPUT["GNSS Receiverへ<br/>RTCM投入"]
    CHECK{"接続継続?"}
    RECONNECT["再接続"]

    START --> CONFIG
    CONFIG --> CONNECT
    CONNECT --> AUTH
    AUTH --> GGA
    GGA --> SEND
    SEND --> RECEIVE
    RECEIVE --> OUTPUT
    OUTPUT --> CHECK
    CHECK -->|"Yes"| GGA
    CHECK -->|"No"| RECONNECT
    RECONNECT --> CONNECT
```

------------------------------------------------------------------------

## 各社サービスの抽象化

利用者側から見ると、docomo、SoftBank、KDDIなどの違いは、可能な限り
**Provider設定** に閉じ込める構成にできます。

``` text
ProviderConfig
├── host
├── port
├── mountpoint
├── username
├── password
└── その他サービス固有設定
```

アプリケーション本体は各社の内部構成を意識せず、共通のNTRIP処理を担当します。

``` mermaid
flowchart LR
    APP["共通 NTRIP Client"]

    D["docomo Adapter<br/>ProviderConfig"]
    S["SoftBank Adapter<br/>ProviderConfig"]
    K["KDDI Adapter<br/>ProviderConfig"]

    APP --> D
    APP --> S
    APP --> K

    D --> DP["docomo RTK Service"]
    S --> SP["SoftBank RTK Service"]
    K --> KP["KDDI RTK Service"]
```

------------------------------------------------------------------------

## 車載システム全体

``` mermaid
flowchart TD
    SAT["GNSS衛星"]
    GNSS["GNSS Receiver"]
    CLIENT["NTRIP Client<br/>車載PC"]
    RTK["RTK Provider<br/>docomo / SoftBank / KDDI等"]
    SENSOR["LiDAR / Camera / IMU等"]
    APP["データ収集アプリ"]
    CLOUD["AWS等<br/>データ収集基盤"]

    SAT -->|"GNSS信号"| GNSS

    GNSS -->|"NMEA-GGA"| CLIENT
    CLIENT -->|"GGA"| RTK
    RTK -->|"RTCM"| CLIENT
    CLIENT -->|"RTCM"| GNSS

    GNSS -->|"RTK FIX位置"| APP
    SENSOR -->|"センサーデータ"| APP
    APP -->|"位置付き計測データ"| CLOUD
```

------------------------------------------------------------------------

## 設計上のポイント

### 意識するもの

-   NTRIP接続先
-   Port
-   Mountpoint
-   ID / Passwordなどの認証情報
-   GNSS ReceiverからのNMEA-GGA取得
-   RTK ProviderへのGGA送信
-   RTCM受信
-   GNSS ReceiverへのRTCM投入
-   切断時の再接続

### 基本的に意識しなくてよいもの

-   RTK Provider内部の基準局
-   基準局の地域分割
-   基準局の選択アルゴリズム
-   上流のNTRIP Server
-   Provider内部のCaster構成
-   基準局間のハンドオーバー
-   VRS/RRSなどProvider内部での補正情報生成処理

------------------------------------------------------------------------

## 最小モデル

システム設計上は、最終的に以下の通信モデルとして扱えます。

``` mermaid
sequenceDiagram
    participant GNSS
    participant Client as NTRIP Client
    participant RTK as RTK Provider

    GNSS->>Client: GGA
    Client->>RTK: GGA
    RTK-->>Client: RTCM
    Client-->>GNSS: RTCM
```

つまり利用者側では、

**GNSS → GGA → NTRIP Client → RTK Provider → RTCM → NTRIP Client →
GNSS**

というデータフローを中心に設計すればよく、各RTK事業者の基準局や上流構成はブラックボックスとして扱えます。
