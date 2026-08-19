# rtk-relay (POC)

[rtk_relay_protocol_design.md](../rtk_relay_protocol_design.md)
の実装。車両からのNTRIPセッションを受け、DynamoDBで認証した上でRTKプロバイダーへ中継する。

## AWSリソースとの関わり

`relay`プロセス自体はステートレスで、認証情報とプロバイダー設定は外部から注入される（ローカル実行は環境変数、AWS実行はDynamoDB/Secrets
Manager）。インフラ全体の構成図は[infra/terraform/README.md](../infra/terraform/README.md)を参照。

``` mermaid
flowchart LR
    VEHICLE["車両 / vehiclesim<br/>NTRIP Client"]
    PROVIDER["RTK Provider /<br/>mockprovider"]

    subgraph RELAY["relay プロセス"]
        AUTH{{"VehicleAuthenticator"}}
        CONF{{"Provider Config"}}
    end

    VEHICLE -->|"GGA (要Basic認証)"| RELAY
    RELAY -->|"RTCM"| VEHICLE
    RELAY -->|"GGA"| PROVIDER
    PROVIDER -->|"RTCM"| RELAY

    AUTH -.->|"ローカル: VEHICLE_API_KEYS"| STATIC["静的マップ<br/>(メモリ内)"]
    AUTH -.->|"AWS: VEHICLE_TABLE_NAME"| DDB["DynamoDB<br/>vehicle_credentials"]
    CONF -.->|"ローカル: PROVIDER_HOST等"| ENV["環境変数"]
    CONF -.->|"AWS: PROVIDER_SECRET_ARN"| SECRETS["Secrets Manager<br/>provider_credentials"]

    RELAY -->|"ログ (JSON, stdout)"| LOGS["CloudWatch Logs<br/>(AWS実行時、awslogsドライバ経由)"]
```

-   `VEHICLE_TABLE_NAME`/`PROVIDER_SECRET_ARN`が未設定ならローカル用の実装（静的マップ／環境変数）にフォールバックする（[internal/auth/vehicle.go](internal/auth/vehicle.go)、[internal/providerconfig/config.go](internal/providerconfig/config.go)）
-   RTCM/GGAの中身はrelayが解釈せずそのまま中継する（[internal/relay/session.go](internal/relay/session.go)）ため、DynamoDB/Secrets
    Managerへのアクセスはセッション確立時の1回のみ

## コマンド

-   `cmd/relay` — 本体。車両向けNTRIP Caster兼プロバイダー向けNTRIP Client
-   `cmd/mockprovider` — ローカル検証用のダミープロバイダー（RTKプロバイダー未選定のため）
-   `cmd/vehiclesim` — ローカル検証用の車両シミュレータ（GGA送信・RTCM受信をログ出力）

## ローカルでの動作確認

``` bash
docker compose up -d mockprovider relay
docker compose run --rm vehiclesim
```

`vehiclesim`のログに`received RTCM bytes`が継続的に出れば、GGA/RTCM双方向の中継が正常に動作している。

## 設定（環境変数）

  変数                    説明
  ----------------------- --------------------------------------------------------------------
  `LISTEN_ADDR`           relayの待受アドレス（例: `:2101`）
  `PROVIDER_SECRET_ARN`   設定時、Secrets Managerからプロバイダー設定(JSON)を取得する（本番/AWS向け）
  `PROVIDER_HOST/PORT/MOUNTPOINT/USERNAME/PASSWORD`   `PROVIDER_SECRET_ARN`未設定時、これらの環境変数から直接読む（ローカル向け）
  `VEHICLE_TABLE_NAME`    設定時、DynamoDBで車両認証を行う（本番/AWS向け）
  `VEHICLE_API_KEYS`      `VEHICLE_TABLE_NAME`未設定時、`vehicleID:apiKey,...`形式で直接指定（ローカル向け）

## AWSへのデプロイ

インフラは`../infra/terraform`で管理する。イメージのビルド・pushは以下（Fargateは`linux/amd64`）。

``` bash
cd relay-server
aws ecr get-login-password --profile fpv-japan --region ap-northeast-1 \
  | docker login --username AWS --password-stdin <account-id>.dkr.ecr.ap-northeast-1.amazonaws.com

docker build --platform linux/amd64 --build-arg CMD=relay \
  -t <account-id>.dkr.ecr.ap-northeast-1.amazonaws.com/rtk-relay:latest .
docker push <account-id>.dkr.ecr.ap-northeast-1.amazonaws.com/rtk-relay:latest

aws ecs update-service --cluster rtk-relay-poc --service rtk-relay-poc \
  --force-new-deployment --profile fpv-japan --region ap-northeast-1
```

車両認証情報の登録例（`api_key_hash`はAPIキーのSHA-256 hex）：

``` bash
printf "<api-key>" | shasum -a 256 | awk '{print $1}'

aws dynamodb put-item \
  --table-name rtk-relay-vehicle-credentials-poc \
  --item '{"vehicle_id": {"S": "vehicle-001"}, "api_key_hash": {"S": "<hash>"}}' \
  --profile fpv-japan --region ap-northeast-1
```

プロバイダー選定後、認証情報は以下でSecrets Managerへ投入する（Terraformはシークレットの値を管理しない）。

``` bash
aws secretsmanager put-secret-value --secret-id <provider_credentials_secret_arn from terraform output> \
  --secret-string '{"host":"...","port":"2101","mountpoint":"...","username":"...","password":"..."}' \
  --profile fpv-japan --region ap-northeast-1
```
