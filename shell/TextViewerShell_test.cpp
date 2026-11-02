#define WIN32_LEAN_AND_MEAN
#include <windows.h>
#include <shobjidl.h>
#include <chrono>
#include <iostream>
#include <string>

static void require(bool condition, const char *message) {
    if (!condition) {
        std::cerr << "FAILED: " << message << "\n";
        ExitProcess(1);
    }
}

int wmain(int argc, wchar_t **argv) {
    require(argc == 4, "expected DLL or --registered, file and directory paths");
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
