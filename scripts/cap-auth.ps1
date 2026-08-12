# cap-auth.ps1 - сохранение токена CrossFit Affiliate Programming для cap-pp-cli.
#
#   .\cap-auth.ps1          пошагово помогает достать токен и сохраняет его
#   .\cap-auth.ps1 status   проверяет, жив ли сохранённый токен
#
# Версия для Windows: то же, что scripts/cap-auth.sh, для PowerShell.
#
# У CAP нет входа по логину и паролю, который можно повторить из терминала:
# кабинет аффилиата использует OAuth2 PKCE и держит короткоживущий токен в
# браузере. Поэтому токен переносится из уже открытой сессии.
#
# Скрипт НИКОГДА не просит пароль и ничего не отправляет наружу: введённый
# токен уходит только в локальный конфиг cap-pp-cli.

param([string]$Command = "")

$ErrorActionPreference = "Stop"
$CapBin = if ($env:CAP_BIN) { $env:CAP_BIN } else { "cap-pp-cli" }
$ToolkitUrl = "https://affiliate.crossfit.com/tools/home"

if (-not (Get-Command $CapBin -ErrorAction SilentlyContinue)) {
    Write-Error "Не найден $CapBin. Распакуйте архив и добавьте папку bin в PATH."
    exit 1
}

function Test-CapToken {
    # Библиотека движений открыта и работает без токена, поэтому живость
    # проверяем именно командой программирования.
    & $CapBin cap day *> $null
    return ($LASTEXITCODE -eq 0)
}

if ($Command -eq "status") {
    if (Test-CapToken) {
        Write-Host "Токен работает: программа CAP читается."
        exit 0
    }
    Write-Host "Токен отсутствует или истёк. Запустите скрипт без параметров."
    exit 4
}

Write-Host @"
Как достать токен (занимает полминуты):

  1. Откройте кабинет аффилиата и войдите, если ещё не вошли.
  2. Нажмите F12 - откроются инструменты разработчика.
  3. Вкладка Application, слева Local Storage, в нём строка
     affiliate.crossfit.com.
  4. Найдите ключ access_token и скопируйте его значение.

Токен живёт недолго - когда команды снова начнут отвечать «token expired»,
повторите эти шаги. Команды по движениям и бенчмаркам работают и без токена.
"@

Start-Process $ToolkitUrl -ErrorAction SilentlyContinue

# Ввод скрытый: токен - это доступ к аккаунту, ему не место в истории консоли.
$Secure = Read-Host -Prompt "Вставьте токен и нажмите Enter" -AsSecureString
$Token = [Runtime.InteropServices.Marshal]::PtrToStringAuto(
    [Runtime.InteropServices.Marshal]::SecureStringToBSTR($Secure))
$Token = $Token -replace '[\s"'']', ''

if ([string]::IsNullOrEmpty($Token)) {
    Write-Error "Токен пустой, ничего не сохранено."
    exit 2
}

& $CapBin auth set-token $Token *> $null
$Token = $null

if (Test-CapToken) {
    Write-Host "Готово: токен сохранён и работает."
} else {
    Write-Error @"
Токен сохранён, но программа не читается.
Скорее всего скопирован не тот ключ - нужен именно access_token,
или срок его действия уже истёк. Повторите запуск скрипта.
"@
    exit 4
}
