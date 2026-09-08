<p align="center">
  <img src="docs/media/wordmark-dark.svg#gh-dark-mode-only" width="380" alt="gadak">
  <img src="docs/media/wordmark-light.svg#gh-light-mode-only" width="380" alt="gadak">
</p>

<p align="center">
  <a href="https://github.com/midagedev/gadak/releases"><img src="https://img.shields.io/github/v/release/midagedev/gadak" alt="Latest Release"></a>
  <a href="https://github.com/midagedev/gadak/actions/workflows/ci.yml"><img src="https://github.com/midagedev/gadak/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-Apache--2.0-blue" alt="License"></a>
</p>

<p align="center"><b>Follow the thread.</b></p>

<p align="center"><sub><a href="README.md">English</a> · <a href="README.ko.md">한국어</a> · 日本語</sub></p>

JQL に `GROUP BY` はありません。「未完了の課題が多いエピックはどれか」を Jira で数えようとすると、
検索 API を 8 ページぶんめくり、返ってきた行を自分のコードで集計することになります。実測で
4,761 ms かかりました。同じ Jira をキャッシュしておけば、同じ答えが SQL 1 本、
22 ms で返ってきます。

gadak は、そのキャッシュを作り、更新し続けるためのツールです。Jira と Confluence にある
課題・コメント・変更履歴・wiki ページがまるごと入り、まとめて検索できるようになります。
キャッシュができたあとの読み取りは、ネットワークに一度も出ません。
バイナリは 1 つで、gadak のアカウントはありません。

## まず動くものを見る

インストールもアカウントも要りません。[ライブデモ](https://gadak.dev/demo/)を開くと、
534 件の課題がブラウザーの中で動きます。

<details>
<summary>▶ 20 秒の GIF (収録は英語)</summary>

<p align="center">
  <img src="docs/media/web-demo.gif" alt="入力するほどリストが絞り込まれ、課題がラベル・優先度・再オープンのバッジと一緒に開き、ドキュメントとボードが同じウィンドウに並ぶ" width="900">
  <br>
  <sub>デモのスナップショットから <a href="e2e/demo/web-demo.spec.ts">e2e/demo/web-demo.spec.ts</a> で生成した 20 秒です。この収録だけは英語で、3 つの言語版で共通です。</sub>
</p>

</details>

## JQL では書けない質問

```bash
gadak sql "select epic_key, count(*) from issues_full where resolved_at is null
           and epic_key <> '' group by epic_key order by 2 desc"
```

エピックごとに未完了の課題を数えるだけの SQL です。同じことを REST API でやるなら、ページを
めくって自分で足すしかありません。ページサイズを超えると、API から返ってくるのは行だけで、
集計は返ってきません。

続きのクエリは [docs/RECIPES.md](docs/RECIPES.md) にあります。上の SQL は、何もインストール
せずに [Datasette Lite でデモのスナップショットに対して実行](<https://lite.datasette.io/?url=https%3A%2F%2Fraw.githubusercontent.com%2Fmidagedev%2Fgadak%2Fmain%2Fexamples%2Fdemo.db#/demo?sql=select+epic_key%2C+count(*)+from+issues_full+where+resolved_at+is+null+and+epic_key+%3C%3E+''+group+by+epic_key+order+by+2+desc>)できます。
SQL を書き換えて、そのまま試せます。

## 計測値

2026-08-26 に、実際に業務で使っている Atlassian Cloud のサイト (課題 3,296 件) に対して
測りました。数値は中央値で、gadak 側は CLI プロセスの起動時間を含みます。

| 質問 | REST API | `gadak` | 倍率 |
| --- | ---: | ---: | ---: |
| 単純なフィルター、課題 100 件 | 583 ms | 19 ms | 31× |
| 課題 1 件と、その全変更履歴 | 710 ms | 28 ms | 25× |
| 全文検索 | 543 ms | 41 ms | 13× |
| **エピックごとの未完了件数 (`GROUP BY`)** | 4,761 ms (API 8 ページをクライアント側で集計) | 22 ms (クエリ 1 本) | **214×** |
| 変更履歴に対するカウント | JQL では表現できない (クロールで約 28 分) | 14 ms | — |
| レート制限 | 429 と Retry-After | なし | — |

gadak が負ける行もあります。最初のフル同期には時間がかかり、キャッシュは同期間隔 1 回ぶん
遅れます。変化のないサイトに対する監視ティックの計測も含めて、測定方法と再測定の履歴は
[docs/BENCHMARKS.md](docs/BENCHMARKS.md) に載せています。

## 手元に残るもの

同じキャッシュを 3 つの入口から使えます。

- **デスクトップアプリ** ([docs/DESKTOP.md](docs/DESKTOP.md))
- **ブラウザーのタブ**。CLI だけ入れて `gadak serve` を実行すると、同じ UI が
  `http://gadak.localhost:7777` に開きます。
- **CLI**。`gadak sql` の結果をパイプで次のコマンドにつなげます。

シェルのないホスト (Claude Desktop など) からは、同じキャッシュを MCP サーバーとして使えます。
画面の表示言語は 3 つ (英語・韓国語・日本語) で、ブラウザーか OS の設定に従い、設定画面で
切り替えられます。

キャッシュの実体は、使っているマシンの中の SQLite ファイル 1 つです。いつ消しても構いません。
ディレクトリを 1 つ消しても失うものはなく、`gadak sync` で作り直せます。書き込みは先に Jira 側へ届き、受け付けられてからキャッシュに
反映されます。

## インストール

### macOS

デスクトップアプリ (CLI 同梱):

```bash
brew install --cask midagedev/tap/gadak
```

CLI だけ入れる場合:

```bash
brew install midagedev/tap/gadak-cli
```

### Windows

デスクトップアプリは [Microsoft Store](https://apps.microsoft.com/detail/9NZW91TXH36G) から
入れてください。Store 側で署名されるので、SmartScreen も Smart App Control も警告を出しません。
Store からインストールすると、`gadak` コマンドも `PATH` に入ります (0.20.2 以降)。

Store を使わず CLI だけ欲しい場合は、[最新リリース](https://github.com/midagedev/gadak/releases/latest)
の `gadak_<version>_windows_amd64.zip` (Arm なら `gadak_<version>_windows_arm64.zip`) を展開し、
`gadak.exe` を `PATH` に置きます。

リリースページのデスクトップ zip (`Gadak-<version>-windows-x64.zip`) には署名がありません。
SmartScreen に止められた場合、それは署名がないという意味で、ウイルスが見つかったという意味では
ありません ([docs/WINDOWS-SIGNING.md](docs/WINDOWS-SIGNING.md))。止められたら Store から
入れ直してください。Smart App Control をオフにする必要はなく、オフにしないでください。

### Jira につなぐ

接続して同期し、`gadak serve` が表示するアドレス (`http://gadak.localhost:7777`) を開きます:

```bash
gadak init && gadak sync && gadak serve
```

必要なのは Jira サイトの [API トークン](https://id.atlassian.com/manage-profile/security/api-tokens)
1 つで、同じサイトの Jira と Confluence の両方に効きます。写す範囲は自分で決めます。Jira は
`--projects`、wiki は `--spaces` で絞り、スペースを指定するまで wiki は同期されません。

### Atlassian のアカウントがない場合

`gadak init --local` で、内蔵トラッカーのワークスペースが作られます。ワークスペースを移すときは
`gadak --workspace <new> migrate --from <old>` で同期済みのデータを運べます。移行先を Linear の
チームにするなら `--to linear` です。

### 2 台目のマシン

ノート PC など 2 台目は、`gadak --workspace laptop init --pairing-code-stdin` でペアリングします。
署名済み dmg、Linux、コンテナでの実行、アップグレードの手順は [docs/INSTALL.md](docs/INSTALL.md)
にあります。

## セキュリティと外部通信

社内の課題データを手元に写すツールなので、導入前に確認したい点をここにまとめます。根拠は
[SECURITY.md](SECURITY.md) が原本で、主張ごとに該当するソースファイルのパスが添えてあります。
接続先ごとの条件とオフにする方法は [docs/NETWORK.md](docs/NETWORK.md) に 1 つずつ書いてあります。

### テレメトリはありません

gadak から外に出る通信は、[SECURITY.md](SECURITY.md) に挙げた 6 か所だけです。5 番と 6 番は
自分でそのコマンドを打ったときにしか起きません。

1. 自分の Atlassian サイト (同期のため)
2. GitHub Releases (1 日 1 回までの匿名バージョン確認。`updateCheck: false` でオフ)
3. Linear (ワークスペースに Linear のソースがあるとき)
4. ペアリングした home 側の `gadak serve` (ペアリングしたときだけ)
5. `gh` (`gadak dev scan` を実行したときだけ)
6. ライブラリのダウンロード (自分で要求したときだけ)

ループバックはこの一覧には入りません。外に出る接続ではなく、自分のマシンの中で待ち受ける
バインドだからです。

インストール時、エラー時、定期実行のどこにも、gadak の運営者へ何かを送る経路はありません。
gadak のサーバーというものが存在しないためです。

### 読み取りはネットワークに出ません

`gadak sql`、`gadak search`、`gadak issue`、MCP のツール、web UI の一覧と詳細は、キャッシュ
だけを読みます。接続を開かないので、レート制限に当たることも、機内でつながらないことも
ありません。例外は接続先に訊く必要がある動詞で、`gadak issue --editmeta` と `gadak fields`
(編集できる項目の問い合わせ)、`gadak api` (素通しのリクエスト)、添付ファイルの表示 (その場で
取得) の 4 つです。

書き込みは手元に溜めません。接続先に届かなかった書き込みは、その場で失敗として返ります。

### 認証情報

**API トークンは、キャッシュ・ログ・スナップショットの 3 か所のどこにも書き込まれません。** キャッシュ
をそのまま誰かに渡しても、その中にトークンは含まれていません。信用しなくてよいこと、
つまり自分のマシンで確かめられることは、確認用のコマンドと一緒に
[docs/PROMISES.md](docs/PROMISES.md) に 12 項目まとめてあります。

### エージェントと外部モデル

**コーディングエージェントにキャッシュを読ませると、読んだ内容はそのエージェントが話している
モデルへ送られます。** gadak からは何も送信されません。エージェントに見せてよい範囲だけを
写すように、`--projects` と `--spaces` で絞ってください。

## コーディングエージェントから使う

gadak がある理由の半分はエージェントです。リファレンスは [docs/MIRROR.md](docs/MIRROR.md)、
ホストごとに 1 つ貼り付ければ済む設定は [docs/AGENT_SETUP.md](docs/AGENT_SETUP.md) にあります。
エージェントがこの上に作ったもの (ダッシュボード、チーム用のテーマ、ランチャー、ライブの MCP
セッション) の録画は [docs/SHOWCASE.md](docs/SHOWCASE.md) に集めています。

### スキルを入れる

```bash
gadak skill install
```

Claude Code にスキーマとクエリのパターンが入ります。追加のプロセスは動きません。別のホストには
名前を付けて同じファイルを入れられます: `gadak skill install codex`、同様に `cursor`、`gemini`、
`opencode`、`grok`。

### MCP で使う

シェルのないホスト (Claude Desktop) には、同じファイルを MCP サーバーとして登録します:

```bash
gadak mcp install claude
```

<p align="center">
  <img src="docs/media/terminal-hero.ja.gif" alt="リストの下に gadak 自身のターミナル。gadak claim NMA-140 で行が進行中に動き、シェルのタブがその課題キーを名前に受け取る。そのシェルで claude が起動し、日本語のプロンプト 1 つでリストが Dana Whitfield の最近動いた課題に変わり、次のプロンプトが同じウィンドウにラベル比率のダッシュボードを保存して開く" width="900">
  <br>
  <sub>シェルはウィンドウの中にあります (⌘K → ターミナル、または Ctrl+`)。<code>gadak claim</code> でシェルと課題が結び付き、タブの名前が課題キーになります。その中で始めたライブの Claude Code セッションが、隣のボードを動かします。1 文目でリストが変わり、2 文目でダッシュボードが開きます。画面からプロンプトまで、すべて日本語のテイクです。プロンプト 2 行のほかに台本はありません。エージェントが作業している区間は早送りです。<a href="e2e/demo/terminal-claude-demo.spec.ts">e2e/demo/terminal-claude-demo.spec.ts</a> を <a href="e2e/demo/record-terminal-claude.sh">record-terminal-claude.sh</a> で収録しました。</sub>
</p>

### 効く 2 つのルール

1. **フィルターは `status_category` と `priority_rank` にかけ、表示名にはかけない。** 表示名は
   アカウントの言語ごとに翻訳されるので、`priority = High` は韓国語のアカウントでは 0 行になり、
   エラーも出ません。
2. **答えは SQL で出し、見せるのはウィンドウに任せる。** `gadak sql --no-header "…" | gadak views open --keys -`
   で、エージェントが出した答えをそのまま自分の画面に並べられます。`gadak views open --jql '…'`
   なら、貼り付けた JQL がチップになって並びます。

### 書き込み

`create`、`edit`、`comment`、`transition`、`claim`、`link`、wiki の `page` 系の動詞は、先に
接続先 (Jira) へ届き、受け付けられてからキャッシュに反映されます。エージェントからの書き込みには、
すべてそのエージェントの名前が付きます。

## 対応しているトラッカー

接続先は 3 つで、動詞は 1 組です。対応している Jira は Cloud です。Server /
Data Center は検証していないため、対応対象にしていません。

- Atlassian Cloud
- Linear (ワークスペース設定の `"linear"` ブロックと `gadak sync --source linear`)
- アプリに同梱の内蔵トラッカー

読み取り、書き込み、階層、wiki、添付、履歴、ボードのレイアウトは 3 つすべてで動きます。接続先
ごとに拒まれる操作は、セルごとにコードの参照を付けた 1 枚の表
[docs/SUPPORT_MATRIX.md](docs/SUPPORT_MATRIX.md) にあります。どの接続先にもないものが 3 つ
あります。UI としてのスプリント、Jira のダッシュボード、Jira の通知受信箱で、これらは Jira 側で
使い続けることになります。

## 向いている場面、向かない場面

向いている場面:

- 毎日の検索を速くしたい
- トラッカーと wiki の両方をエージェントに読ませたい
- オフラインでも読みたい

向かない場面:

- スプリント計画と管理作業
- UI の中でページを編集したい
- 1 分の遅れが問題になる

詳しくは [docs/CONCEPT.md](docs/CONCEPT.md#good-fit-bad-fit)、他のツールとの比較は
[docs/FAQ.md](docs/FAQ.md#how-it-compares) にあります。

## いまの状態

> **状態: 0.21、まだ 0.x です。** 同期、読み取り API、write-through の書き込み、デスクトップ、
> ウェブ、CLI、MCP は、実際のサイトに対して検証しています。変更履歴は
> [CHANGELOG.md](CHANGELOG.md) にあり、英語で公開しています。

0.x の間に互換性を約束している範囲は、次の 3 つです。原本は
[specs/000-product/data-model.md](specs/000-product/data-model.md) にあります。

1. `issues_full` と [docs/RECIPES.md](docs/RECIPES.md) のクエリ
2. `gadak sql` の標準出力の形式
3. `gadak views open --keys -` の意味

メンテナーは現在 1 人です。それを踏まえて判断してください。開発が止まったとしても、手元に
残るのは普通の SQLite ファイルと Apache-2.0 のコードで、Jira 側のデータには何も起きません。
難しい質問への答えは [docs/FAQ.md](docs/FAQ.md) にあります。

## ドキュメント

- [CHANGELOG.md](CHANGELOG.md): 何が出たか。英語で公開しています (韓国語版は [CHANGELOG.ko.md](CHANGELOG.ko.md))
- [docs/INSTALL.md](docs/INSTALL.md) · [docs/DESKTOP.md](docs/DESKTOP.md): インストール、初回起動、デスクトップアプリ
- [docs/MIRROR.md](docs/MIRROR.md) · [docs/MCP.md](docs/MCP.md) · [docs/AGENT_SETUP.md](docs/AGENT_SETUP.md): SQL、CLI、REST、MCP と、ホスト別の設定
- [docs/RECIPES.md](docs/RECIPES.md) · [docs/DASHBOARDS.md](docs/DASHBOARDS.md): JQL では書けない質問の SQL と、エージェントが書いたダッシュボード
- [docs/SHOWCASE.md](docs/SHOWCASE.md): ウィンドウ、ランチャー、エージェントの録画
- [SECURITY.md](SECURITY.md) · [docs/NETWORK.md](docs/NETWORK.md) · [docs/PROMISES.md](docs/PROMISES.md): 脅威モデル、すべての接続先、確認用のコマンド
- [docs/BENCHMARKS.md](docs/BENCHMARKS.md) · [docs/SUPPORT_MATRIX.md](docs/SUPPORT_MATRIX.md): 計測方法と、接続先ごとの対応表
- [docs/FAQ.md](docs/FAQ.md) · [docs/MAINTENANCE.md](docs/MAINTENANCE.md): よくある質問、リリースと issue の扱い、誰が保守しているか
- [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) · [docs/decisions/0003-local-process.md](docs/decisions/0003-local-process.md): 仕組みと、ローカルプロセスが必要な理由
- [docs/EXTENDING.md](docs/EXTENDING.md): フォークせずに自分の環境に合わせる方法
- [docs/WINDOWS-SIGNING.md](docs/WINDOWS-SIGNING.md): Windows の署名について
- [docs/README.md](docs/README.md): そのほかのドキュメント

## バグ報告とコントリビューション

報告は [GitHub issue](https://github.com/midagedev/gadak/issues) へお願いします。メンテナーが
[公開バックログ](https://gadak.dev/backlog/)へ写し、コミットメッセージの `GDK-nnn` は
そこで開けます。バグ報告には次の 3 つを入れてください。

1. Jira のデプロイ種別 (Cloud)
2. gadak のコミット
3. 実行したコマンド

公開の issue には、実際の課題データを貼らないでください。API トークンとサイトの URL も同じです。
エージェントと一緒に使っていて引っかかったときは、投げた質問と、エージェントがしたことをそのまま
issue に書いてください。コードで参加するなら [.github/CONTRIBUTING.md](.github/CONTRIBUTING.md) と
[docs/project/GOOD_FIRST_ISSUES.md](docs/project/GOOD_FIRST_ISSUES.md) から。次に作る機能が
なぜそれなのかは、出典付きで [docs/project/THEORY.md](docs/project/THEORY.md) にあります。

## ライセンス

Apache-2.0 です。`LICENSE` と `NOTICE` を参照してください。
