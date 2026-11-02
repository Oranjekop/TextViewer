#define WIN32_LEAN_AND_MEAN
#include <windows.h>
#include <shobjidl.h>
#include <chrono>
#include <iostream>

static void require(bool condition, const char *message) {
    if (!condition) {
        std::cerr << "FAILED: " << message << "\n";
        ExitProcess(1);
    }
}

int wmain(int argc, wchar_t **argv) {
    require(argc == 4, "expected DLL, file and directory paths");
    require(SUCCEEDED(CoInitializeEx(nullptr, COINIT_APARTMENTTHREADED)), "initialize COM");
    HMODULE library = LoadLibraryW(argv[1]);
    require(library != nullptr, "load extension without extra runtime DLLs");
    using GetClass = HRESULT(WINAPI *)(REFCLSID, REFIID, void **);
    using CanUnload = HRESULT(WINAPI *)();
    auto getClass = reinterpret_cast<GetClass>(GetProcAddress(library, "DllGetClassObject"));
    auto canUnload = reinterpret_cast<CanUnload>(GetProcAddress(library, "DllCanUnloadNow"));
    require(getClass && canUnload, "COM exports");
    require(canUnload() == S_OK, "initial unload state");
    const CLSID id = {0x9dfeef63, 0x8b31, 0x4bd2, {0x85, 0xa0, 0xa5, 0x36, 0x87, 0xce, 0x7c, 0x96}};
    IClassFactory *factory = nullptr;
    require(SUCCEEDED(getClass(id, IID_IClassFactory, reinterpret_cast<void **>(&factory))), "class factory");
    IExplorerCommand *command = nullptr;
    require(SUCCEEDED(factory->CreateInstance(nullptr, IID_IExplorerCommand, reinterpret_cast<void **>(&command))), "IExplorerCommand");
    require(canUnload() == S_FALSE, "live COM objects prevent unloading");
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
    CoTaskMemFree(icon);
    command->Release();
    factory->Release();
    require(canUnload() == S_OK, "final unload state");
    FreeLibrary(library);
    CoUninitialize();
    std::cout << "Explorer command tests passed\n";
    return 0;
}
