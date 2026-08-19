# rtk-relay (POC)

[rtk_relay_protocol_design.md](../rtk_relay_protocol_design.md)
の実装。車両からのNTRIPセッションを受け、DynamoDBで認証した上でRTKプロバイダーへ中継する。

## 今どこで何が動いているか

| プログラム | 役割 | 今の実体 | ローカルで起動する必要があるのは |
|---|---|---|---|
| `cmd/relay` | 車両からはサーバ役／プロバイダーへはクライアント役（二重ロール） | AWS ECS（`rtk-relay-poc`）で**常時稼働中** | コードを変更し、AWSへ再デプロイする前に手元で動作確認したいとき |
| `cmd/mockprovider` | サーバ役（`net.Listen`で待ち受けるだけ）＝RTKプロバイダーの代役 | AWS ECS（`rtk-relay-mockprovider-poc`）で**常時稼働中**（プロバイダー未選定のための暫定。[rtk_provider_comparison.md](../rtk_provider_comparison.md)で決まったら`infra/terraform`の`deploy_mockprovider = false`で撤去する） | AWSに繋がずオフラインで素早く検証したいとき |
| `cmd/vehiclesim` | クライアント役（自分から`net.Dial`で接続しにいく）＝車両の代役 | **常時稼働のサービスとしてはどこにもデプロイしていない**。手元やCIなど好きな場所で都度実行する検証ツール | 動作確認したいとき全般（`RELAY_ADDR`でローカル/AWSどちらのrelayにも向けられる） |

**重要**：ローカルの`relay`/`mockprovider`を起動するかどうかは、AWS上で常時動いている`rtk-relay-poc`/`rtk-relay-mockprovider-poc`には一切影響しない。両者は同じコードから作られた別々のデプロイ先（自分のマシン上のプロセス
vs
ECS上のFargateタスク）であり、独立して動いている。どちらと通信するかは`vehiclesim`（や実車のNTRIP
Client）が接続する`RELAY_ADDR`が`localhost:2101`か`rtk.fpv.jp:2101`かで決まる。

### AWS上のコンテナ構成

``` mermaid
flowchart LR
    VEHICLESIM["vehiclesim<br/>(ローカル/CI, 都度実行)<br/>常時デプロイなし"]

    subgraph CLUSTER["ECS Cluster: rtk-relay-poc"]
        direction LR
        subgraph SVC1["ECSサービス<br/>rtk-relay-poc"]
            RELAYC["コンテナ: relay<br/>(cmd/relay)<br/>:2101 / Public IP"]
        end
        subgraph SVC2["ECSサービス<br/>rtk-relay-mockprovider-poc"]
            MOCKC["コンテナ: mockprovider<br/>(cmd/mockprovider)<br/>:2201 / 内部限定"]
        end
    end

    ECR["ECR: rtk-relay<br/>タグ latest(relay) /<br/>mockprovider"]

    VEHICLESIM -->|"rtk.fpv.jp:2101<br/>(NLB経由)"| RELAYC
    RELAYC -->|"VPC内プライベートIP:2201<br/>(relay_task SGのみ許可)"| MOCKC
    ECR -.->|"docker pull"| RELAYC
    ECR -.->|"docker pull"| MOCKC
```

同じECRリポジトリ（`rtk-relay`）から、タグ違いのイメージを2つのECSサービスとしてそれぞれ動かしている。`relay`はNLB経由でインターネットから`rtk.fpv.jp:2101`で到達可能だが、`mockprovider`は`relay_task`のセキュリティグループからしか到達できず、インターネットには非公開。

> `mockprovider`タスクを再作成するとVPC内プライベートIPが変わる（Fargateの仕様上、固定できない）。Secrets
> Managerの`provider_credentials`に古いIPを設定したままだと`relay`が`no route to host`で502を返すので、`mockprovider`を再デプロイしたら[アドレスを取り直してSecretsを更新](#awsへのデプロイ)する必要がある（恒久対応するならService
> Discoveryの導入を検討）。

したがって、AWS上の動作を確認したいだけなら、ローカルでは何も起動せず以下を実行するだけでよい（`vehicle-001`はDynamoDBに登録済みのテスト車両、[AWSへのデプロイ](#awsへのデプロイ)参照）。

``` bash
cd relay-server
RELAY_ADDR=rtk.fpv.jp:2101 VEHICLE_ID=vehicle-001 VEHICLE_API_KEY=dev-key-001 MOUNTPOINT=RELAY \
  go run ./cmd/vehiclesim
```

`session established with relay`のあと`received RTCM bytes`が継続的に出れば、AWS上のrelay/mockprovider共々正常に動作している。

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
-   `cmd/mockprovider` — RTKプロバイダー未選定のためのダミープロバイダー（ローカル・AWS両方で動かせる）
-   `cmd/vehiclesim` — 車両シミュレータ（GGA送信・RTCM受信をログ出力）。常時稼働のサービスではなく、検証したいときに都度実行するツール

それぞれが「今どこで動いているか」は[今どこで何が動いているか](#今どこで何が動いているか)を参照。

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

### mockprovider再デプロイ後のアドレス更新

`mockprovider`を再デプロイ（`force-new-deployment`等）するとVPC内プライベートIPが変わるため、`relay`が参照しているSecretsの値を更新しないと`502 Bad Gateway`になる（[AWS上のコンテナ構成](#aws上のコンテナ構成)参照）。

``` bash
TASK_ARN=$(aws ecs list-tasks --cluster rtk-relay-poc --service-name rtk-relay-mockprovider-poc \
  --profile fpv-japan --region ap-northeast-1 --query 'taskArns[0]' --output text)
PRIVATE_IP=$(aws ecs describe-tasks --cluster rtk-relay-poc --tasks "$TASK_ARN" \
  --profile fpv-japan --region ap-northeast-1 \
  --query 'tasks[0].attachments[0].details[?name==`privateIPv4Address`].value' --output text)

aws secretsmanager put-secret-value --secret-id <provider_credentials_secret_arn from terraform output> \
  --secret-string "{\"host\":\"$PRIVATE_IP\",\"port\":\"2201\",\"mountpoint\":\"TESTMOUNT\",\"username\":\"relay\",\"password\":\"relay-secret\"}" \
  --profile fpv-japan --region ap-northeast-1

aws ecs update-service --cluster rtk-relay-poc --service rtk-relay-poc \
  --force-new-deployment --profile fpv-japan --region ap-northeast-1
```

（`relay`はSecretsを起動時にしか読まないため、最後の`update-service`で`relay`自体も再デプロイして反映させる）
