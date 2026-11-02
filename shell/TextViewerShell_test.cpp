#define WIN32_LEAN_AND_MEAN
#include <windows.h>
#include <shobjidl.h>
#include <shlobj.h>
#include <chrono>
#include <iostream>
#include <string>
#include <vector>

static void require(bool condition, const char *message) {
    if (!condition) {
        std::cerr << "FAILED: " << message << "\n";
        ExitProcess(1);
    }
}

static IShellItemArray *selection(const std::vector<std::wstring> &paths) {
    std::vector<PIDLIST_ABSOLUTE> ids(paths.size(), nullptr);
    for (size_t i = 0; i < paths.size(); ++i) {
        require(SUCCEEDED(SHParseDisplayName(paths[i].c_str(), nullptr, &ids[i], 0, nullptr)), "parse selected path");
    }
    IShellItemArray *items = nullptr;
    require(SUCCEEDED(SHCreateShellItemArrayFromIDLists(
        static_cast<UINT>(ids.size()), const_cast<PCIDLIST_ABSOLUTE *>(ids.data()), &items)), "multiple selection array");
    for (auto id : ids) CoTaskMemFree(id);
    return items;
}

static void checkInvocation(IExplorerCommand *command, const std::wstring &file, const std::wstring &capture) {
    std::vector<std::wstring> paths(600, file);
    auto *items = selection(paths);
    require(SUCCEEDED(command->Invoke(items, nullptr)), "invoke selected files in isolated recorder");
    items->Release();
    const auto deadline = std::chrono::steady_clock::now() + std::chrono::seconds(10);
    size_t captured = 0, batches = 0;
    while (std::chrono::steady_clock::now() < deadline) {
        captured = 0;
        batches = 0;
        WIN32_FIND_DATAW entry = {};
        HANDLE search = FindFirstFileW((capture + L"\\*.args").c_str(), &entry);
        if (search != INVALID_HANDLE_VALUE) {
            do {
                HANDLE input = CreateFileW((capture + L"\\" + entry.cFileName).c_str(), GENERIC_READ, FILE_SHARE_READ,
                                           nullptr, OPEN_EXISTING, FILE_ATTRIBUTE_NORMAL, nullptr);
                if (input == INVALID_HANDLE_VALUE) continue;
                const DWORD bytes = GetFileSize(input, nullptr);
                std::wstring content(bytes / sizeof(wchar_t), L'\0');
                DWORD read = 0;
                const bool ok = ReadFile(input, content.data(), bytes, &read, nullptr) && read == bytes;
                CloseHandle(input);
                require(ok, "read recorded paths");
                size_t start = 0, end;
                while ((end = content.find(L'\n', start)) != std::wstring::npos) {
                    require(content.substr(start, end - start) == file, "spaces and Unicode passed intact");
                    ++captured;
                    start = end + 1;
                }
                ++batches;
            } while (FindNextFileW(search, &entry));
            FindClose(search);
        }
        if (captured == paths.size()) break;
        Sleep(10);
    }
    require(captured == paths.size(), "all selected paths reached the recorder");
    require(batches > 1, "long selection split into multiple command lines");
    std::cout << "Recorded " << captured << " paths in " << batches << " batches\n";
}

int wmain(int argc, wchar_t **argv) {
    require(argc == 4 || argc == 6, "expected DLL or --registered, file and directory paths, optional --invoke capture-directory");
    require(SUCCEEDED(CoInitializeEx(nullptr, COINIT_APARTMENTTHREADED)), "initialize COM");
    const bool registered = wcscmp(argv[1], L"--registered") == 0;
    HMODULE library = nullptr;
    using GetClass = HRESULT(WINAPI *)(REFCLSID, REFIID, void **);
    using CanUnload = HRESULT(WINAPI *)();
    CanUnload canUnload = nullptr;
    const CLSID id = {0x9dfeef63, 0x8b31, 0x4bd2, {0x85, 0xa0, 0xa5, 0x36, 0x87, 0xce, 0x7c, 0x96}};
    IClassFactory *factory = nullptr;
    IExplorerCommand *command = nullptr;
    if (registered) {
        const HRESULT hr = CoCreateInstance(id, nullptr, CLSCTX_LOCAL_SERVER, IID_PPV_ARGS(&command));
        if (FAILED(hr)) std::cerr << "CoCreateInstance HRESULT: 0x" << std::hex << hr << "\n";
        require(SUCCEEDED(hr), "activate the registered packaged COM command");
    } else {
        library = LoadLibraryW(argv[1]);
        require(library != nullptr, "load extension without extra runtime DLLs");
        auto getClass = reinterpret_cast<GetClass>(GetProcAddress(library, "DllGetClassObject"));
        canUnload = reinterpret_cast<CanUnload>(GetProcAddress(library, "DllCanUnloadNow"));
        require(getClass && canUnload, "COM exports");
        require(canUnload() == S_OK, "initial unload state");
        require(SUCCEEDED(getClass(id, IID_IClassFactory, reinterpret_cast<void **>(&factory))), "class factory");
        require(SUCCEEDED(factory->CreateInstance(nullptr, IID_IExplorerCommand, reinterpret_cast<void **>(&command))), "IExplorerCommand");
        require(canUnload() == S_FALSE, "live COM objects prevent unloading");
    }
    LPWSTR title = nullptr;
    require(SUCCEEDED(command->GetTitle(nullptr, &title)), "command title");
    require(wcscmp(title, L"\u5728 TextViewer \u4e2d\u6253\u5f00") == 0, "localized command title");
    CoTaskMemFree(title);
    EXPCMDSTATE state = ECS_ENABLED;
    require(SUCCEEDED(command->GetState(nullptr, FALSE, &state)) && state == ECS_HIDDEN, "no selection is hidden");
    for (int i = 2; i < 4; ++i) {
        IShellItem *item = nullptr;
        require(SUCCEEDED(SHCreateItemFromParsingName(argv[i], nullptr, IID_PPV_ARGS(&item))), "shell item");
        IShellItemArray *items = nullptr;
        require(SUCCEEDED(SHCreateShellItemArrayFromShellItem(item, IID_PPV_ARGS(&items))), "selection array");
        state = ECS_HIDDEN;
        require(SUCCEEDED(command->GetState(items, FALSE, &state)), "command state");
        require(state == (i == 2 ? ECS_ENABLED : ECS_HIDDEN), "file enabled and folder hidden");
        if (i == 2) {
            const auto start = std::chrono::steady_clock::now();
            for (int n = 0; n < 10000; ++n) {
                require(SUCCEEDED(command->GetState(items, FALSE, &state)), "repeated command state");
            }
            const auto elapsed = std::chrono::duration<double, std::micro>(std::chrono::steady_clock::now() - start);
            std::cout << "GetState average: " << elapsed.count() / 10000 << " us\n";
        }
        items->Release();
        item->Release();
    }
    for (bool mixed : {false, true}) {
        auto *items = selection({argv[2], mixed ? argv[3] : argv[2]});
        state = ECS_HIDDEN;
        require(SUCCEEDED(command->GetState(items, FALSE, &state)), "multiple selection state");
        require(state == (mixed ? ECS_HIDDEN : ECS_ENABLED), "multiple files enabled and mixed folders hidden");
        if (mixed) require(command->Invoke(items, nullptr) == E_INVALIDARG, "mixed selection rejected by Invoke");
        items->Release();
    }
    if (argc == 6) {
        require(!registered && wcscmp(argv[4], L"--invoke") == 0, "invocation only allowed with isolated recorder");
        checkInvocation(command, argv[2], argv[5]);
    }
    LPWSTR icon = nullptr;
    require(SUCCEEDED(command->GetIcon(nullptr, &icon)), "icon location");
    require(wcsstr(icon, L"TextViewer.exe,0") != nullptr, "icon points at editor");
    const std::wstring executable(icon, wcslen(icon) - 2);
    require(GetFileAttributesW(executable.c_str()) != INVALID_FILE_ATTRIBUTES, "registered editor still exists");
    std::wcout << L"Editor: " << executable << L"\n";
    CoTaskMemFree(icon);
    command->Release();
    if (factory) factory->Release();
    if (canUnload) require(canUnload() == S_OK, "final unload state");
    if (library) FreeLibrary(library);
    CoUninitialize();
    std::cout << "Explorer command tests passed\n";
    return 0;
}
