#define WIN32_LEAN_AND_MEAN
#include <windows.h>
#include <shobjidl.h>
#include <shlwapi.h>
#include <shellapi.h>
#include <atomic>
#include <new>
#include <string>
#include <vector>

// Explorer only loads this small native COM handler. The Go editor starts
// after Invoke, never while Explorer is constructing a context menu.
static HMODULE moduleHandle;
static std::atomic<long> liveObjects{0};
static const CLSID commandID = {0x9dfeef63, 0x8b31, 0x4bd2, {0x85, 0xa0, 0xa5, 0x36, 0x87, 0xce, 0x7c, 0x96}};

static std::wstring editorPath() {
    wchar_t path[32768];
    const DWORD n = GetModuleFileNameW(moduleHandle, path, ARRAYSIZE(path));
    if (n == 0 || n >= ARRAYSIZE(path)) return {};
    std::wstring result(path, n);
    const auto slash = result.find_last_of(L"\\/");
    if (slash == std::wstring::npos) return {};
    result.resize(slash + 1);
    result += L"TextViewer.exe";
    return result;
}

static HRESULT launchEditor(const std::wstring &exe, std::wstring arguments) {
    STARTUPINFOW startup = {sizeof(startup)};
    startup.dwFlags = STARTF_USESHOWWINDOW;
    startup.wShowWindow = SW_SHOWNORMAL;
    PROCESS_INFORMATION process = {};
    if (!CreateProcessW(exe.c_str(), arguments.data(), nullptr, nullptr, FALSE, 0,
                        nullptr, nullptr, &startup, &process)) {
        return HRESULT_FROM_WIN32(GetLastError());
    }
    CloseHandle(process.hThread);
    CloseHandle(process.hProcess);
    return S_OK;
}

class OpenCommand final : public IExplorerCommand {
    std::atomic<ULONG> refs{1};
public:
    OpenCommand() { ++liveObjects; }
    ~OpenCommand() { --liveObjects; }
    IFACEMETHODIMP QueryInterface(REFIID id, void **out) override {
        if (!out) return E_POINTER;
        *out = nullptr;
        if (IsEqualIID(id, IID_IUnknown) || IsEqualIID(id, IID_IExplorerCommand)) {
            *out = static_cast<IExplorerCommand *>(this);
            AddRef();
            return S_OK;
        }
        return E_NOINTERFACE;
    }
    IFACEMETHODIMP_(ULONG) AddRef() override { return ++refs; }
    IFACEMETHODIMP_(ULONG) Release() override {
        const ULONG n = --refs;
        if (!n) delete this;
        return n;
    }
    IFACEMETHODIMP GetTitle(IShellItemArray *, LPWSTR *out) override {
        return SHStrDupW(L"\u5728 TextViewer \u4e2d\u6253\u5f00", out);
    }
    IFACEMETHODIMP GetIcon(IShellItemArray *, LPWSTR *out) override {
        if (!out) return E_POINTER;
        const auto path = editorPath();
        if (path.empty()) { *out = nullptr; return E_FAIL; }
        return SHStrDupW((path + L",0").c_str(), out);
    }
    IFACEMETHODIMP GetToolTip(IShellItemArray *, LPWSTR *out) override {
        if (out) *out = nullptr;
        return E_NOTIMPL;
    }
    IFACEMETHODIMP GetCanonicalName(GUID *out) override {
        if (!out) return E_POINTER;
        *out = commandID;
        return S_OK;
    }
    IFACEMETHODIMP GetState(IShellItemArray *items, BOOL, EXPCMDSTATE *out) override {
        if (!out) return E_POINTER;
        *out = ECS_HIDDEN;
        DWORD count = 0;
        if (!items || FAILED(items->GetCount(&count)) || count == 0) return S_OK;
        SFGAOF filesystem = 0, folders = 0;
        if (SUCCEEDED(items->GetAttributes(SIATTRIBFLAGS_AND, SFGAO_FILESYSTEM, &filesystem))
            && (filesystem & SFGAO_FILESYSTEM)
            && SUCCEEDED(items->GetAttributes(SIATTRIBFLAGS_OR, SFGAO_FOLDER, &folders))
            && !(folders & SFGAO_FOLDER)) {
            *out = ECS_ENABLED;
        }
        return S_OK;
    }
    IFACEMETHODIMP Invoke(IShellItemArray *items, IBindCtx *) override {
        if (!items) return E_INVALIDARG;
        DWORD count = 0;
        if (FAILED(items->GetCount(&count)) || count == 0) return E_INVALIDARG;
        EXPCMDSTATE state = ECS_HIDDEN;
        if (FAILED(GetState(items, TRUE, &state)) || state != ECS_ENABLED) return E_INVALIDARG;
        const auto exe = editorPath();
        if (exe.empty()) return E_FAIL;
        const std::wstring prefix = L"\"" + exe + L"\" --";
        std::wstring args = prefix;
        std::vector<std::wstring> batches;
        for (DWORD i = 0; i < count; ++i) {
            IShellItem *item = nullptr;
            HRESULT hr = items->GetItemAt(i, &item);
            if (FAILED(hr)) return hr;
            PWSTR path = nullptr;
            hr = item->GetDisplayName(SIGDN_FILESYSPATH, &path);
            item->Release();
            if (FAILED(hr)) return hr;
            const std::wstring argument = L" \"" + std::wstring(path) + L"\"";
            CoTaskMemFree(path);
            // CreateProcess includes the executable and terminating NUL in
            // its limit. Split large selections without dropping any path.
            if (prefix.size() + argument.size() >= 32767) {
                return HRESULT_FROM_WIN32(ERROR_FILENAME_EXCED_RANGE);
            }
            if (args.size() + argument.size() >= 32767) {
                batches.push_back(std::move(args));
                args = prefix;
            }
            args += argument;
        }
        batches.push_back(std::move(args));
        for (auto &batch : batches) {
            const HRESULT hr = launchEditor(exe, std::move(batch));
            if (FAILED(hr)) return hr;
        }
        return S_OK;
    }
    IFACEMETHODIMP GetFlags(EXPCMDFLAGS *out) override {
        if (!out) return E_POINTER;
        *out = ECF_DEFAULT;
        return S_OK;
    }
    IFACEMETHODIMP EnumSubCommands(IEnumExplorerCommand **out) override {
        if (out) *out = nullptr;
        return E_NOTIMPL;
    }
};

class CommandFactory final : public IClassFactory {
    std::atomic<ULONG> refs{1};
public:
    CommandFactory() { ++liveObjects; }
    ~CommandFactory() { --liveObjects; }
    IFACEMETHODIMP QueryInterface(REFIID id, void **out) override {
        if (!out) return E_POINTER;
        *out = nullptr;
        if (IsEqualIID(id, IID_IUnknown) || IsEqualIID(id, IID_IClassFactory)) {
            *out = static_cast<IClassFactory *>(this);
            AddRef();
            return S_OK;
        }
        return E_NOINTERFACE;
    }
    IFACEMETHODIMP_(ULONG) AddRef() override { return ++refs; }
    IFACEMETHODIMP_(ULONG) Release() override {
        const ULONG n = --refs;
        if (!n) delete this;
        return n;
    }
    IFACEMETHODIMP CreateInstance(IUnknown *outer, REFIID id, void **out) override {
        if (!out) return E_POINTER;
        *out = nullptr;
        if (outer) return CLASS_E_NOAGGREGATION;
        auto *command = new (std::nothrow) OpenCommand();
        if (!command) return E_OUTOFMEMORY;
        const HRESULT hr = command->QueryInterface(id, out);
        command->Release();
        return hr;
    }
    IFACEMETHODIMP LockServer(BOOL lock) override {
        liveObjects += lock ? 1 : -1;
        return S_OK;
    }
};

STDAPI DllGetClassObject(REFCLSID id, REFIID iid, void **out) {
    if (!out) return E_POINTER;
    *out = nullptr;
    if (!IsEqualCLSID(id, commandID)) return CLASS_E_CLASSNOTAVAILABLE;
    auto *factory = new (std::nothrow) CommandFactory();
    if (!factory) return E_OUTOFMEMORY;
    const HRESULT hr = factory->QueryInterface(iid, out);
    factory->Release();
    return hr;
}

STDAPI DllCanUnloadNow() {
    return liveObjects == 0 ? S_OK : S_FALSE;
}

BOOL WINAPI DllMain(HINSTANCE instance, DWORD reason, LPVOID) {
    if (reason == DLL_PROCESS_ATTACH) {
        moduleHandle = instance;
        DisableThreadLibraryCalls(instance);
    }
    return TRUE;
}
