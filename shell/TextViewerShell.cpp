#define WIN32_LEAN_AND_MEAN
#include <windows.h>
#include <shobjidl.h>
#include <shlwapi.h>
#include <shellapi.h>
#include <atomic>
#include <new>
#include <string>

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
        if (!items || FAILED(items->GetCount(&count)) || count != 1) return S_OK;
        SFGAOF attrs = 0;
        if (SUCCEEDED(items->GetAttributes(SIATTRIBFLAGS_AND, SFGAO_FILESYSTEM | SFGAO_FOLDER, &attrs))
            && (attrs & SFGAO_FILESYSTEM) && !(attrs & SFGAO_FOLDER)) {
            *out = ECS_ENABLED;
        }
        return S_OK;
    }
    IFACEMETHODIMP Invoke(IShellItemArray *items, IBindCtx *) override {
        if (!items) return E_INVALIDARG;
        DWORD count = 0;
        if (FAILED(items->GetCount(&count)) || count != 1) return E_INVALIDARG;
        IShellItem *item = nullptr;
        HRESULT hr = items->GetItemAt(0, &item);
        if (FAILED(hr)) return hr;
        PWSTR path = nullptr;
        hr = item->GetDisplayName(SIGDN_FILESYSPATH, &path);
        item->Release();
        if (FAILED(hr)) return hr;
        const auto exe = editorPath();
        const std::wstring args = L"-- \"" + std::wstring(path) + L"\"";
        CoTaskMemFree(path);
        if (exe.empty()) return E_FAIL;
        SHELLEXECUTEINFOW launch = {sizeof(launch)};
        launch.fMask = SEE_MASK_NOASYNC | SEE_MASK_FLAG_NO_UI;
        launch.lpVerb = L"open";
        launch.lpFile = exe.c_str();
        launch.lpParameters = args.c_str();
        launch.nShow = SW_SHOWNORMAL;
        return ShellExecuteExW(&launch) ? S_OK : HRESULT_FROM_WIN32(GetLastError());
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
