# RTK中継サーバ Terraform（POC）

[rtk_relay_server_requirements.md](../../rtk_relay_server_requirements.md)のAWS構成案の実装。ECR
+ ECS/Fargate + NLB(TCP) + DynamoDB + Secrets Managerを作成する。デフォルトVPCを利用し、NAT
Gatewayコストを避けるためFargateタスクにPublic
IPを直接割り当てている（POC構成であり、本番では専用VPC・プライベートサブネット化を検討）。

## 前提

-   `~/.aws/credentials`に`fpv-japan`プロファイルが設定済みであること
-   このプロファイルのIAMユーザーにECS/ECR/EC2(VPC)/IAM/DynamoDB/SecretsManager/ELBの権限が付与されていること

## 使い方

``` bash
terraform init
terraform plan
terraform apply
```

`apply`直後はECRにイメージが存在しないため、ECSサービスのタスクは起動に失敗する。`../relay-server/README.md`の手順でイメージをpushし、`--force-new-deployment`でタスクを再作成すること。

## 主な出力

-   `ecr_repository_url` — イメージのpush先
-   `nlb_dns_name` — 車両からの接続先（DNS設定の対象）
-   `vehicle_credentials_table_name` / `provider_credentials_secret_arn` — 認証情報投入先

## DNS（Value-Domain, fpv.jp）

TerraformはRoute53を使わず、AWS側は`nlb_dns_name`を出力するのみ。Value-Domainのコントロールパネルで、`var.relay_subdomain`（デフォルト`rtk-relay.fpv.jp`）に対して以下のCNAMEを手動で追加する。

    種別:  CNAME
    ホスト: rtk-relay
    値:    <terraform output nlb_dns_name>

NLBのDNS名はサービス再作成では変わらないが、NLB自体を作り直した場合は変わるため、その際はCNAMEの再設定が必要。

## 破棄

検証が終わったら費用発生を止めるため destroy する。

``` bash
terraform destroy
```
