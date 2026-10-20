# dch — devcontainer helper

一個單一 Go binary 的 Dev Container 管理工具，適合在 Linux / WSL2 / macOS 的 terminal 使用。只需 Go 標準函式庫，沒有額外 Go dependency。

## 前置需求

- Docker CLI + 可存取的 Docker daemon
- 官方 [Dev Container CLI](https://github.com/devcontainers/cli)：`npm install -g @devcontainers/cli` (或官方安裝腳本)
- 建置原始碼使用 Go 1.21+（下載預先建置的 Linux binary 則不需要 Go）
- 專案位置必須有 `<project>/.devcontainer/devcontainer.json`

## 安裝

```bash
go build -o dch .
mkdir -p ~/.local/bin
install -m 755 dch ~/.local/bin/dch
# 確認 ~/.local/bin 在 PATH：
export PATH="$HOME/.local/bin:$PATH"
```

## 使用

```bash
dch config   # Add project / Remove project / Done：加入時輸入 project path，空白 = 當前目錄
dch list     # 顯示所有註冊項目及狀態，再透過選單查看單一項目詳細資訊
dch build    # 選專案，再選「完整重建+重建容器」或「只 build image」
dch up       # 選專案，devcontainer up
dch connect  # 選 RUNNING 專案，用 devcontainer exec 開啟 shell
```

互動選單：方向鍵上下 + Enter 選擇，`q` / `Esc` 取消；非 TTY 時自動改為編號選單。

### `~/.dch/config` 格式

內容是 JSON（無 `.json` 副檔名），只存**專案路徑**；名稱每次從 `.devcontainer/devcontainer.json` 讀取（支援 JSONC 的註解與尾逗號），若 `name` 未設定，使用資料夾名稱。

```json
{
  "version": 1,
  "projects": [
    "/home/alanli/Projects/aol-svr-external",
    "/home/alanli/Projects/another-app"
  ]
}
```

`dch config` 的 Remove **只會移除註冊紀錄**，不刪除 container、image 或 workspace。

### Build 的兩種模式

| 模式 | 對應指令 | 效果 |
| --- | --- | --- |
| 完整重建（預設第一個選項） | `devcontainer up --workspace-folder PATH --remove-existing-container --build-no-cache` | 刪除現有 devcontainer 並無快取重建，**會啟動**；需要再次確認。container writable layer 內的未持久化變更可能遺失。 |
| 只 build image | `devcontainer build --workspace-folder PATH --no-cache` | 只產生 image，不會替換已在執行的 container；下次 `up` 未必使用剛 build 的 image。 |

## Status

`dch list` 使用 `docker ps -a` 取得帶 `devcontainer.local_folder` label 的容器，再用 `docker inspect` 比對 exact workspace path 與 `devcontainer.config_file`；不是根據 container name 猜測。

狀態：`RUNNING`、`STOPPED`、`NOT CREATED`、`CONFIG MISSING/INVALID`，Docker 不可用時顯示 `UNKNOWN` 並提示錯誤。若同專案有多個容器會附上數量。

`dch connect` 走 `devcontainer exec`，會使用 Dev Container CLI 的 `remoteUser`、`remoteEnv` 設定。優先使用 bash，沒有 bash 時改為 sh。專案如果 `STOPPED`，請先 `dch up`。

## 限制

- 只支援專案根目錄的 `.devcontainer/devcontainer.json`，不含多個並列的設定檔或只存放 `.devcontainer.json` 的形式。
- 狀態比對使用本機絕對路徑；若專案透過不同 symlink/Windows VS Code 路徑建立容器，Docker label 可能不一致，狀態無法匹配。
- 官方 `devcontainer` CLI 支援 Docker Compose devcontainer，但 Docker label 的行為依設定而異；沒有 matching label 的 Compose service 可能不會出現在 `dch list`。
- 未實作 Docker stop/down（避免超出需求的破壞性操作）。

## 測試

```bash
go test ./...
```
