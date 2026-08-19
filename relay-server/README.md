# rtk-relay (POC)

[rtk_relay_protocol_design.md](../rtk_relay_protocol_design.md)
の実装。車両からのNTRIPセッションを受け、DynamoDBで認証した上でRTKプロバイダーへ中継する。

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
