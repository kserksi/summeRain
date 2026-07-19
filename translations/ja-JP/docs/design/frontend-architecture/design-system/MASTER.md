# デザインシステム - MASTER（summeRain）

> [!WARNING]
> **アーカイブ済みの設計記録です。** このページは完成済みの V2 フロントエンドより前に
> 作成されており、古いバージョン、パス、実装状態を含む場合があります。

> この文書は、フロントエンド UI/UX の**唯一の信頼できる情報源**でした。
> [10-pages-ui-ux.md](../10-pages-ui-ux.md) の全ページはここから派生し、ページ固有の
> 例外は `design-system/pages/<page>.md` に置いて本ファイルより優先する方針でした。
> このシステムは `mockup/index.html` のプロトタイプで検証済みです。

## 1. スタイル方針：Warm Soft Studio（maia）

- shadcn **maia** プリセットの構造
  （`npx shadcn@latest apply --preset b3RZAU6YV`）を土台にします。**プリセットの色、書体、
  アイコンは無視**し、以下のコーヒートークンで置き換えます。
- **カード、視覚的に独立した要素、大きな角丸**を重視し、鋭い要素を避けます。
  `inverted-translucent` と控えめなアクセントを持つ**半透明のナビゲーションバー**を使用します。
- 落ち着き、専門性、暖かさを保ちます。Linear、Vercel、Stripe を参考にしつつ、冷たい青を
  暖かなコーヒー色に置き換えます。コンテンツを優先し、1 画面につき主要 CTA は 1 つ、装飾は最小限とします。
- コンポーネントライブラリ：**shadcn/ui**。セクション 5 を参照。

## 2. カラートークン（コーヒー）

**ライト（モカ）**

| トークン | 値 | 用途 |
|---|---|---|
| `--bg` | `#F1E7DA` | ページ背景 |
| `--bg2` | `#EADFCF` | セカンダリサーフェス、入力欄の背景、表の見出し |
| `--card` | `#FFFCF8` | カード |
| `--text` | `#33261B` | 本文 |
| `--muted` | `#6E5C49` | 補助テキスト。カードに対して 6.24:1 |
| `--border` | `#DAC7AE` | 境界線と区切り線 |
| `--primary` | `#6F4E37` | プライマリー色（モカ） |
| `--primaryHover` | `#573D2B` | プライマリーのホバー色 |
| `--primarySoft` | `#EFE2D2` | 淡いプライマリー背景 |
| `--accent` | `#A9764F` | 補助色とグラデーション |
| `--success/--danger/--info/--warn` | `#5C7A4A / #A8412F / #4A6E92 / #B9742A` | 柔らかく調整した状態色 |

**ダーク（エスプレッソ）**

| トークン | 値 |
|---|---|
| `--bg / --bg2 / --card` | `#16100D / #1E1611 / #251B14` |
| `--text / --muted` | `#F2E7D6 / #C5B59E` |
| `--border` | `#4D3A29` |
| `--primary / --primaryHover / --primarySoft` | `#D4A57E / #B9895F / #33261829` |
| `--accent` | `#C39A72` |
| `success/danger/info/warn` | `#9CBF82 / #D88A78 / #86A8C9 / #E0AC64` |

- 本文と背景のコントラストは **11.98:1**、補助テキストとカードは **6.24:1** です。
  いずれも AA の 4.5:1 を超えます。
- **コンポーネント内で生の 16 進数カラー値を使用しません。** `bg-primary` や
  `text-muted-foreground` などのセマンティックトークンクラスを使用します。ダークモードはトークン
  で自動調整し、**手動の `dark:` 上書きを書きません**。

## 3. タイポグラフィ

- [07 外部リソースのローカル化](../07-production-standards.md)を満たすため、Web フォントを
  読み込まず**システムフォントスタック**を使用します。
- 文字サイズ：12 / 14 / 16 / 18 / 24 / 32 / 48。本文は **16**、行の高さは **1.65**。
- 文字の太さ：本文 400 / ラベル 500 / 見出し 600-700。
- 数値列、価格、タイマーには `tabular-nums` を使い、レイアウトのずれを防ぎます。
- 見出しは流動的な `clamp()` を使います。例：ランディングページの H1
  `clamp(32px,6vw,58px)`。

## 4. 角丸 / 影 / 間隔

- **角丸（maia の大サイズ）：** カード 22-24px / コントロール 12-14px / 大型コンテナー 28-30px /
  ピル 999px。
- **影：** 暖かい茶を基調とした多層の柔らかな影。ダークモードでは重い影を
  低輝度の輪郭線に置き換えます。
- **間隔：** 4/8 の基準（4 / 8 / 12 / 16 / 24 / 32 / 48）。

## 5. コンポーネント（shadcn/ui）と構成規則

**一覧：** フォーム
`Field/FieldGroup/FieldLabel/Input/InputGroup/Select/Switch/ToggleGroup/Slider/Textarea`、
データ `Card`（完全構成）/ `Table/Badge/Avatar/Progress/Chart`、ナビゲーション
`NavigationMenu/Tabs/Breadcrumb/Pagination`、オーバーレイ
`Dialog/AlertDialog/Sheet/Drawer/DropdownMenu/Tooltip/Popover`、フィードバック
`sonner/Alert/Skeleton/Spinner/Empty/Progress`、コマンド `Command`、レイアウト
`Separator/ScrollArea`。

**必須の構成規則：**

- フォームは必ず `FieldGroup` + `Field` で構成し、生の div + `space-y` は使用しません。
  検証時は `Field` に `data-invalid`、コントロールに `aria-invalid` を付けます。
- 間隔は `gap-*` を使い、`space-x/y-*` は使いません。同じ縦横寸法には `size-*`、
  条件付きクラスには `cn()` を使います。
- 空状態は `Empty`、メッセージは `Alert`、Toast は `sonner`、区切り線は
  `Separator`、プレースホルダーは `Skeleton`、状態は `Badge` を使用します。
  **装飾した div による代用品を独自に作りません。**
- 破壊的操作は `AlertDialog` で確認します。すべての `Dialog/Sheet` に `Title` を含め、
  視覚的に隠す場合は `sr-only` を使用します。
- `Card` は完全な `CardHeader/Title/Description/Content/Footer` 構成を使用します。
- オーバーレイコンポーネントに**手書きの z-index を設定しません**。重なり順はコンポーネント自身が
  管理します。

**アイコン：** 単一のアウトラインスタイルの **Tabler Icons**（`@tabler/icons-react`）を使用します。
`Button` 内では `data-icon="inline-start|end"` を使い、**サイズクラスを追加しません**。
アイコンボタンは 44x44 以上で `aria-label` を持たせます。`components.json` の
`iconLibrary` を **tabler** に設定し、プリセットの Phosphor アイコンを置き換えます。

**外観を統一する場合はネイティブコントロールを使わない：** ネイティブの `<select>` をカスタム
ドロップダウンに置き換え、カスタムスクロールバーを使い、モバイルでは表をカードに変換します。

## 6. 統一状態基準

`hover`（トークンを明るく）/ `active`（拡大率 .97-.98）/ `disabled`（不透明度 .5 + カーソル）/
`focus-visible`（2-4px のリング）/ `loading`（スピナー + 無効化）/ `empty`
（`Empty` + 文言 + CTA）/ `error`（文言 + 再試行）/ `skeleton`（>300ms）。

## 7. レスポンシブ動作

- **モバイルファースト**。ブレークポイントは 375 / 768 / 1024 / 1440。
- 幅 820 以下：ナビゲーションバー -> **ハンバーガーメニュー + スライド式ドロワー**、表 -> **カード**、すべてのグリッドを
  1 カラムに積み、文字と間隔を狭めます。
- 見出しには `clamp()` を使用し、**横スクロールを発生させません**。
- タッチ対象は 44x44 以上。デスクトップのコンテナーは `max-w-7xl`。

## 8. モーション

- マイクロインタラクションは 150-300ms。開始は `ease-out`、終了は `ease-in`。
  **`prefers-reduced-motion` を尊重**し、アニメーションとトランジションを全体で無効にします。
  **幅／高さはアニメーションさせません。**
- 登場時は `fadeUp` + カードの段階表示。カード／ボタンはホバーで浮かせ、進捗バーは
  きらめかせ、読み込み中アイコンは回転させ、ヒーローのドット模様には `breathe` を適用します。
- ページ遷移時にフェードインします。

## 9. テーマ切り替え（View Transitions を優先、マスクでフォールバック）

- 上部バーに ☀/🌙 の**明示的な**切り替えを置き、メニュー内に隠しません。`light` / `dark` は
  トークンで適応します。
- **View Transitions 対応時**（Chrome/Edge）、`::view-transition-new(root)` に
  `clip-path: circle()` を使い、切り替えボタンの中心から **0 から 150%** へ拡大します。
  **実際の新テーマの内容**を円形に切り替える最良の効果です。
- **View Transitions 非対応時：** `.theme-mask` を使用します。300vmax の単色円を
  Web Animations API で `transform:scale(0 to 1)` をアニメーションさせます。
- **`prefers-reduced-motion`** の場合はアニメーションせず即時切り替えます。
- localStorage に永続化します。初回描画はシステムの `prefers-color-scheme` に従い、
  インラインの事前描画スクリプトでちらつきを防ぎます。

## 10. ライトボックス（詳細画像）

詳細画像はクリック可能で「クリックして拡大」というヒントを表示します。クリックすると、
拡大画像、閉じるボタン、背景幕を持つ全画面ライトボックスを開きます。ボタン、背景幕の
クリック、Esc で閉じられ、開始／終了には拡大縮小のトランジションを使用します。

## 11. アクセシビリティとパフォーマンス

- コントラストは 4.5:1（AA）以上。フォーカスリングを表示し、Tab の順序と視覚上の順序を一致
  させ、アイコンボタンに `aria-label`、画像に `alt` を付け、セマンティックな見出し階層を
  使用します。
- 画像は `loading="lazy"` と寸法プレースホルダーで CLS を防止します。ルートは遅延読み込みし、
  50 項目以上の一覧は仮想化します。

---

<- [索引](../README.md) - ページ仕様：
[10-pages-ui-ux.md](../10-pages-ui-ux.md)
