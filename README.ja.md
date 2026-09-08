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

<p align="center"><sub><a href="README.md">English</a> · <a href="README.ko.md">한국어</a> · 日本語 — 英語版が原本で、この文書は英語版と一緒に更新されます。</sub></p>

自分の Jira を、ローカルの SQLite ファイルひとつにします。「止まっているエピックはどれ?」が、
訊けない質問ではなく、クエリ 1 行になります。

gadak は Jira *と* Confluence を、課題・コメント・履歴・wiki ページまで、このコンピューターの
SQLite ファイルひとつにミラーします。すべてがひとつのインデックスに入り、検索にネットワークは
要りません。[デスクトップアプリ](docs/DESKTOP.md)かブラウザーのタブでトリアージするか、
コーディングエージェントに SQL で訊かせて、同じウィンドウに答えを出させてください。
バイナリーはひとつ、gadak のアカウントはありません。

**ミラーは捨ててよいキャッシュです。** このプロジェクトが明日止まっても、ディレクトリーを
ひとつ削除すれば終わりで、失うものはありません。正本は Jira にあります。

<p align="center">
  <a href="https://gadak.dev/demo/"><b>▶&nbsp; ライブデモを開く</b></a>
  &nbsp;—&nbsp; 534 件の課題を、いまブラウザーで。
  <br>
  <a href="CHANGELOG.md">Changelog</a>
  &nbsp;—&nbsp; 何が出たか (英語)。
</p>

## インストール

macOS アプリ、CLI 同梱:

```bash
brew install --cask midagedev/tap/gadak
```

CLI だけ入れて、同じ UI を `gadak serve` でブラウザーのタブに開くなら:

```bash
brew install midagedev/tap/gadak-cli
```

Jira に接続してから、`gadak serve` が表示するアドレス (`http://gadak.localhost:7777`) を
開きます:

```bash
gadak init && gadak sync && gadak serve
```

Jira サイトには [API トークン](https://id.atlassian.com/manage-profile/security/api-tokens)が
ひとつ必要で、同じサイトの Jira と Confluence の両方に使われます。**何をミラーするかは自分で
選びます。** Jira は `--projects` で、wiki は `--spaces` で絞り込み、wiki はスペースを指定する
までオフのままです。Atlassian アカウントがないなら、`gadak init --local` が内蔵トラッカーの
ワークスペースを作ります。あとから `gadak --workspace <new> migrate --from <old>` で同期済みの
ミラーをそこへ運べますし、`--to linear` なら Linear のチームへ運べます。

**Windows:** デスクトップアプリは [Microsoft Store](https://apps.microsoft.com/detail/9NZW91TXH36G)
にあります。Store が署名するので SmartScreen も Smart App Control も止めず、Store からの
インストールは `gadak` コマンドも `PATH` に載せます (0.20.2 から)。Store を使わず CLI だけ
なら、[最新リリース](https://github.com/midagedev/gadak/releases/latest)から
`gadak_<version>_windows_amd64.zip` (または `arm64`) を取り、展開して `gadak.exe` を `PATH`
に置きます。リリースのデスクトップ zip (`Gadak-<version>-windows-x64.zip`) は未署名のままです。
SmartScreen が止めるのは署名がないからで、ウイルス検出ではありません
([理由](docs/WINDOWS-SIGNING.md))。止められたら Store からインストールしてください。
Smart App Control をオフにはしないでください。

ウィンドウの表示は英語・韓国語・日本語です。ブラウザーか OS の言語に従い、設定で切り替えられます。

署名済みの dmg、Linux の tarball、2 台目のペアリング
(`gadak --workspace laptop init --pairing-code-stdin`)、Docker、アップグレードは
[`docs/INSTALL.md`](docs/INSTALL.md) にあります。

## 要点

```bash
gadak sql "select epic_key, count(*) from issues_full where resolved_at is null
           and epic_key <> '' group by epic_key order by 2 desc"
```

JQL に `GROUP BY` はありません。「本当に止まっているエピックはどれか」は難しい質問ではなく、
データがファイルになるまでは訊けない質問です。[`docs/RECIPES.md`](docs/RECIPES.md) に続きが
あり、[Datasette Lite はこのクエリをデモのスナップショット上でブラウザーの中で実行します](<https://lite.datasette.io/?url=https%3A%2F%2Fraw.githubusercontent.com%2Fmidagedev%2Fgadak%2Fmain%2Fexamples%2Fdemo.db#/demo?sql=select+epic_key%2C+count(*)+from+issues_full+where+resolved_at+is+null+and+epic_key+%3C%3E+''+group+by+epic_key+order+by+2+desc>)。
何もインストールせずに。

2026-08-26 に実際の Cloud サイトで測定 (課題 3,296 件、中央値、CLI の起動時間を含む):

| 質問 | REST API | `gadak` | |
| --- | ---: | ---: | ---: |
| 単純なフィルター、課題 100 件 | 583 ms | 19 ms | 31× |
| 課題 1 件と、その全履歴 | 710 ms | 28 ms | 25× |
| 全文検索 | 543 ms | 41 ms | 13× |
| **エピックごとの未完了件数 (`GROUP BY`)** | 4,761 ms — API 8 ページをクライアント側で集計 | 22 ms — クエリ 1 本 | **214×** |
| 変更履歴に対するカウント | 表現できない — クロールに約 28 分 | 14 ms | — |

ページサイズを超えると、JQL の答えは遅くなるのではなく、訊けなくなります。API は行を渡すだけで、
集計は渡しません。測定方法、再測定の履歴、そして gadak が負ける行 (最初のフル同期、静かな
サイトでの監視ティック、同期間隔ひとつ分の遅れ) は [`docs/BENCHMARKS.md`](docs/BENCHMARKS.md)
にあります。

<details>
<summary>▶ 紙のリストを 20 秒で (GIF)</summary>

<p align="center">
  <img src="docs/media/web-demo.gif" alt="入力するほど紙のリストが絞り込まれ、課題がラベル・優先度・再オープンのバッジと一緒に開き、ドキュメントとボードが同じウィンドウにある" width="900">
  <br>
  <sub>ウィンドウを 20 秒で。<a href="e2e/demo/web-demo.spec.ts">e2e/demo/web-demo.spec.ts</a> がデモのスナップショットから生成しました。</sub>
</p>

</details>

> **状態: 0.21、まだ 0.x です。** 同期、読み取り API、書き込み (write-through)、デスクトップ、
> ウェブ、CLI、MCP は実際のサイトに対して検証されています。[`CHANGELOG.md`](CHANGELOG.md)。

## エージェントのために

gadak がある理由の半分はこれです。リファレンス: **[docs/MIRROR.md](docs/MIRROR.md)**。
ホストごとに貼り付けひとつ: [`docs/AGENT_SETUP.md`](docs/AGENT_SETUP.md)。

```bash
gadak skill install
```

スキーマとクエリのパターンが入り、追加のプロセスはありません。これは Claude Code 向けの
インストールです。別のホストを名指しすれば同じファイルがそこに入ります: `gadak skill install codex`、
同様に cursor、gemini、opencode、grok。シェルのないホスト (Claude Desktop) には、同じミラーが
MCP サーバーになります:

```bash
gadak mcp install claude
```

<p align="center">
  <img src="docs/media/terminal-hero.ja.gif" alt="リストの下に gadak 自身のターミナル。gadak claim NMA-140 で行が進行中に動き、シェルのタブがそのキーを名前に受け取る。そのシェルで claude が起動し、日本語のプロンプトひとつでリストが Dana Whitfield の最近動いた課題に変わり、次のプロンプトが同じウィンドウにラベル比率のダッシュボードを保存して開く" width="900">
  <br>
  <sub>シェルはウィンドウの中にあります (⌘K → ターミナル、または Ctrl+`)。<code>gadak claim</code> がシェルを課題に結び、タブの名前が課題キーになります。その中で始めたライブの Claude Code セッションが隣のボードを動かします。一文がリストになり、次の一文がダッシュボードを描きます。画面・トラッカー・プロンプトのすべてが日本語のテイクで、プロンプト 2 行のほかに台本はありません。エージェントが作業している区間は早送りです。<a href="e2e/demo/terminal-claude-demo.spec.ts">e2e/demo/terminal-claude-demo.spec.ts</a> を <a href="e2e/demo/record-terminal-claude.sh">record-terminal-claude.sh</a> で収録しました。</sub>
</p>

価値の大半は 2 つのルールにあります。フィルターは `status_category` と `priority_rank` に
かけ、表示名にはかけないこと。Jira は表示名をアカウントごとに翻訳するので、`priority = High`
は韓国語のサイトでは黙って 0 行になります。そして SQL が答え、ウィンドウが見せること:
`gadak sql --no-header "…" | gadak views open --keys -` はエージェントの答えをあなたの画面に
置き、`gadak views open --jql '…'` は貼り付けた JQL をチップとして着地させます。書き込み
(`create`、`edit`、`comment`、`transition`、`claim`、`link`、wiki の `page` 動詞) はミラーが
更新される前に正本を通り、エージェントの書き込みにはすべてエージェントの名前が付きます。

エージェントがこの上に作ったもの (ダッシュボード、チームのテーマ、ランチャー、ライブの MCP
セッション) は録画のギャラリーにあります: [`docs/SHOWCASE.md`](docs/SHOWCASE.md)。

**あなたのミラーを読むエージェントは、読んだものを自分が話すモデルへ送ります。** gadak 自体は
何も送りません ([`SECURITY.md`](SECURITY.md))。エージェントに見せてよい範囲にミラーを絞って
ください。gadak がネットワークに触れる場所 (同期、書き込み、ペアリング) は
[`docs/NETWORK.md`](docs/NETWORK.md) がすべての接続とそのオフスイッチを一つずつ説明します。

## 対応範囲

3 つの正本、1 組の動詞: Atlassian Cloud、Linear (ワークスペース設定の `"linear"` ブロックと
`gadak sync --source linear`)、そしてアプリと一緒に持ち歩く内蔵トラッカー。読み取り、書き込み、
階層、wiki、添付、履歴、ボードのレイアウトは 3 つすべてで動きます。各正本が何を拒むかは、
すべてのセルにコードの引用を付けた表ひとつにあります: [`docs/SUPPORT_MATRIX.md`](docs/SUPPORT_MATRIX.md)。
どの正本にもないものが 3 つ: UI としてのスプリント、Jira のダッシュボード、Jira の通知受信箱。
これらは Jira に残ります。

## そのほか

**向く場面 / 向かない場面。** 毎日の検索の速さ、トラッカー *と* wiki の上のエージェント、
オフラインでの読み取りには向きます。スプリント計画、管理作業、UI のページエディター、1 分の
遅れも許せない場面は Jira に残してください。[`docs/CONCEPT.md`](docs/CONCEPT.md#good-fit-bad-fit)。

**仕組み。** バイナリーひとつ、SQLite ファイルひとつ。差分同期に、突き合わせのパスを加えます。
[`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md)。拡張機能や Forge アプリにしなかった理由:
[`docs/decisions/0003-local-process.md`](docs/decisions/0003-local-process.md)。

**比較。** jira-cli はコマンドごとにライブの API と話します。Rovo MCP も両方のソースを
検索しますが、ホスト型です: 集計はなく、オフラインもなく、呼び出しごとにトークンを使います。
[`docs/FAQ.md`](docs/FAQ.md#how-it-compares)。

**自分のものにする。** 設定、エンリッチメント、SQL の 2 軸で、フォークはいりません:
[`docs/EXTENDING.md`](docs/EXTENDING.md)。

## ドキュメント

- [`CHANGELOG.md`](CHANGELOG.md) — 何が出たか (英語 / [한국어](CHANGELOG.ko.md))
- [`docs/INSTALL.md`](docs/INSTALL.md) · [`docs/DESKTOP.md`](docs/DESKTOP.md) — インストール、初回起動、デスクトップアプリ
- [`docs/SHOWCASE.md`](docs/SHOWCASE.md) — ウィンドウ、ランチャー、その両方を動かすエージェントを、カメラで
- [`docs/MIRROR.md`](docs/MIRROR.md) · [`docs/MCP.md`](docs/MCP.md) · [`docs/AGENT_SETUP.md`](docs/AGENT_SETUP.md) — SQL、CLI、REST、MCP、ホストごとに貼り付けひとつ
- [`docs/RECIPES.md`](docs/RECIPES.md) · [`docs/DASHBOARDS.md`](docs/DASHBOARDS.md) — JQL では訊けない質問を SQL で。エージェントが書いたダッシュボード
- [`SECURITY.md`](SECURITY.md) · [`docs/FAQ.md`](docs/FAQ.md) · [`MAINTENANCE.md`](docs/MAINTENANCE.md) — 脅威モデル、サイトへの負荷、誰が保守しているか
- [`docs/README.md`](docs/README.md) — そのほかのドキュメント

## 誰が作っているか

いまは 1 人です。それを天秤に載せてください。反対側にはこうあります: ミラーは自分の Jira の
捨ててよいキャッシュで、0.x の契約は [data-model.md](specs/000-product/data-model.md) の 3 つの
約束 (`issues_full` と RECIPES のクエリ、`gadak sql` の標準出力、`gadak views open --keys -`)、
ライセンスは Apache-2.0、ファイルはただの SQLite です。難しい質問は
[`docs/FAQ.md`](docs/FAQ.md) に。信用しなくてよいことは、それを確かめるコマンドと一緒に
[`PROMISES.md`](docs/PROMISES.md) に。

## コントリビューションとフィードバック

[`CONTRIBUTING.md`](.github/CONTRIBUTING.md) と、始めるなら
[`docs/project/GOOD_FIRST_ISSUES.md`](docs/project/GOOD_FIRST_ISSUES.md)。次の機能がなぜそれなのかは、
出典つきで [`docs/project/THEORY.md`](docs/project/THEORY.md)。
バグ報告には Jira のデプロイ種別 (Cloud)、gadak のコミット、実行したコマンドが必要です。
実際の課題データ、トークン、サイトの URL を公開の issue に貼らないでください。コミットの
`GDK-nnn` キーは[公開バックログ](https://gadak.dev/backlog/)で解決します。何かを報告するには
[GitHub issue](https://github.com/midagedev/gadak/issues) を開いてください。メンテナーがそこへ
ミラーします。エージェントと一緒に gadak を使っていて引っかかったら、訊いた質問とエージェントが
したことを issue に書いてください。

## ライセンス

Apache-2.0。`LICENSE` と `NOTICE` を参照してください。
