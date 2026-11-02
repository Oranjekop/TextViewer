#define WIN32_LEAN_AND_MEAN
#define WINVER 0x0A00
#define _WIN32_WINNT 0x0A00
#include <windows.h>
#include <filesystem>
#include <fstream>
#include <iostream>
#include <iterator>
#include <stdexcept>
#include <string>
#include <vector>

static void require(bool condition, const char *message) {
    if (!condition) throw std::runtime_error(message);
}

struct IconPixels {
    int width, height;
    std::vector<DWORD> pixels;
};

static IconPixels readIcon(HICON icon) {
    require(icon != nullptr, "The native window has no explicit icon");
    ICONINFO info = {};
    require(GetIconInfo(icon, &info) != FALSE, "GetIconInfo failed");
    BITMAP bitmap = {};
    const bool haveColor = info.hbmColor &&
        GetObjectW(info.hbmColor, sizeof(bitmap), &bitmap) == sizeof(bitmap);
    if (!haveColor) {
        if (info.hbmColor) DeleteObject(info.hbmColor);
        if (info.hbmMask) DeleteObject(info.hbmMask);
        throw std::runtime_error("The window icon has no color bitmap");
    }
    IconPixels result{bitmap.bmWidth, bitmap.bmHeight,
                      std::vector<DWORD>(bitmap.bmWidth * bitmap.bmHeight)};
    BITMAPINFO format = {};
    format.bmiHeader.biSize = sizeof(BITMAPINFOHEADER);
    format.bmiHeader.biWidth = result.width;
    format.bmiHeader.biHeight = -result.height;
    format.bmiHeader.biPlanes = 1;
    format.bmiHeader.biBitCount = 32;
    format.bmiHeader.biCompression = BI_RGB;
    HDC dc = GetDC(nullptr);
    const int rows = GetDIBits(dc, info.hbmColor, 0, result.height,
                              result.pixels.data(), &format, DIB_RGB_COLORS);
    ReleaseDC(nullptr, dc);
    DeleteObject(info.hbmColor);
    if (info.hbmMask) DeleteObject(info.hbmMask);
    require(rows == result.height, "GetDIBits failed");
    return result;
}

static HICON windowIcon(HWND window, WPARAM kind) {
    DWORD_PTR result = 0;
    require(SendMessageTimeoutW(window, WM_GETICON, kind, 0,
                SMTO_ABORTIFHUNG, 2000, &result) != 0, "WM_GETICON timed out");
    return reinterpret_cast<HICON>(result);
}

static void verifyIcon(HICON actual, HMODULE resources,
                       int width, int height) {
    const auto pixels = readIcon(actual);
    require(pixels.width == width && pixels.height == height,
            "Window icon dimensions do not match the current DPI");
    HICON expected = reinterpret_cast<HICON>(LoadImageW(resources,
        MAKEINTRESOURCEW(1), IMAGE_ICON, width, height, 0));
    require(expected != nullptr, "Cannot load the EXE's prefiltered icon resource");
    const auto source = readIcon(expected);
    DestroyIcon(expected);
    require(pixels.pixels == source.pixels,
            "Window icon pixels do not match the EXE's prefiltered app artwork");
}

struct Child {
    PROCESS_INFORMATION process = {};
    HWND window = nullptr;

    ~Child() {
        if (!process.hProcess) return;
        if (window) PostMessageW(window, WM_CLOSE, 0, 0);
        if (WaitForSingleObject(process.hProcess, 5000) == WAIT_TIMEOUT) {
            // Only this test's isolated child can be terminated.
            TerminateProcess(process.hProcess, 99);
            WaitForSingleObject(process.hProcess, 5000);
        }
        CloseHandle(process.hThread);
        CloseHandle(process.hProcess);
    }
};

static BOOL CALLBACK findWindow(HWND window, LPARAM context) {
    auto &child = *reinterpret_cast<Child *>(context);
    DWORD pid = 0;
    GetWindowThreadProcessId(window, &pid);
    wchar_t name[128] = {};
    GetClassNameW(window, name, ARRAYSIZE(name));
    if (pid == child.process.dwProcessId && IsWindowVisible(window) &&
        std::wstring(name) == L"MyGoWindow") {
        child.window = window;
        return FALSE;
    }
    return TRUE;
}

static void verifyLaunch(const std::wstring &exe, const std::wstring &desktopName,
                         HDESK desktop, const std::filesystem::path &fixture,
                         HMODULE resources) {
    std::ofstream file(fixture);
    file << "TextViewer native window icon regression\n";
    file.close();
    require(file.good(), "Cannot write isolated text fixture");
    Child child;
    STARTUPINFOW startup = {sizeof(startup)};
    startup.lpDesktop = const_cast<LPWSTR>(desktopName.c_str());
    startup.dwFlags = STARTF_USESHOWWINDOW;
    startup.wShowWindow = SW_SHOWNORMAL;
    std::wstring command = L"\"" + exe + L"\" -- \"" + fixture.wstring() + L"\"";
    require(CreateProcessW(exe.c_str(), command.data(), nullptr, nullptr, FALSE,
                0, nullptr, nullptr, &startup, &child.process) != FALSE,
            "Cannot start the editor on the private desktop");
    bool loaded = false;
    for (int i = 0; i < 300; ++i) {
        EnumDesktopWindows(desktop, findWindow, reinterpret_cast<LPARAM>(&child));
        if (child.window) {
            wchar_t title[1024] = {};
            GetWindowTextW(child.window, title, ARRAYSIZE(title));
            if (std::wstring(title).find(fixture.filename().wstring()) != std::wstring::npos) {
                loaded = true;
                break;
            }
        }
        if (WaitForSingleObject(child.process.hProcess, 50) != WAIT_TIMEOUT) break;
    }
    require(loaded, "The editor did not open the isolated file");
    const UINT dpi = GetDpiForWindow(child.window);
    require(dpi != 0, "Cannot read native window DPI");
    const HICON small = windowIcon(child.window, ICON_SMALL);
    const HICON big = windowIcon(child.window, ICON_BIG);
    verifyIcon(small, resources, GetSystemMetricsForDpi(SM_CXSMICON, dpi),
                                GetSystemMetricsForDpi(SM_CYSMICON, dpi));
    verifyIcon(big, resources, GetSystemMetricsForDpi(SM_CXICON, dpi),
                              GetSystemMetricsForDpi(SM_CYICON, dpi));
    // Ordinary moves/resizes must not repeatedly decode and allocate icons.
    require(SetWindowPos(child.window, nullptr, 60, 60, 920, 680,
                         SWP_NOACTIVATE | SWP_NOZORDER) != FALSE, "Cannot resize test window");
    require(GetDpiForWindow(child.window) == dpi, "Unexpected monitor DPI change");
    require(windowIcon(child.window, ICON_SMALL) == small &&
            windowIcon(child.window, ICON_BIG) == big,
            "An ordinary move/resize recreated the window icons");
    std::wcout << fixture.extension().wstring() << L": EXE icon resource pixels passed; dpi="
               << dpi << L", small=" << GetSystemMetricsForDpi(SM_CXSMICON, dpi)
               << L", big=" << GetSystemMetricsForDpi(SM_CXICON, dpi) << L"\n";
    require(PostMessageW(child.window, WM_CLOSE, 0, 0) != FALSE, "Cannot close test window");
    require(WaitForSingleObject(child.process.hProcess, 5000) == WAIT_OBJECT_0,
            "The isolated editor did not exit normally");
    DWORD exitCode = 0;
    require(GetExitCodeProcess(child.process.hProcess, &exitCode) && exitCode == 0,
            "The isolated editor failed");
    child.window = nullptr;
}

int wmain(int argc, wchar_t **argv) {
    if (argc != 3) {
        std::cerr << "usage: window-tests TextViewer.exe assets/icon.png\n";
        return 2;
    }
    HDESK desktop = nullptr;
    HMODULE resources = nullptr;
    std::filesystem::path isolated;
    try {
        std::ifstream input(std::filesystem::path(argv[2]), std::ios::binary);
        std::vector<BYTE> png{std::istreambuf_iterator<char>(input), {}};
        require(input.is_open() && !input.bad() && png.size() > 24 && png.size() < 4 * 1024 * 1024,
                "Cannot read the source PNG");
        const auto dimension = [&](int offset) {
            return (DWORD(png[offset]) << 24) | (DWORD(png[offset + 1]) << 16) |
                   (DWORD(png[offset + 2]) << 8) | DWORD(png[offset + 3]);
        };
        require(dimension(16) >= 256 && dimension(16) == dimension(20),
                "The app icon source must be square and at least 256 pixels");
        resources = LoadLibraryExW(argv[1], nullptr,
            LOAD_LIBRARY_AS_DATAFILE | LOAD_LIBRARY_AS_IMAGE_RESOURCE);
        require(resources != nullptr, "Cannot read EXE icon resources");
        // Use the EXE's prefiltered frames as an independent reference. The
        // old raw-PNG native scaling path can have valid handles and sizes
        // while still showing visibly different pixels at 16px and 32px.
        for (UINT dpi : {96u, 144u, 192u, 240u, 288u}) {
            for (int metric : {SM_CXSMICON, SM_CXICON}) {
                const int size = GetSystemMetricsForDpi(metric, dpi);
                HICON icon = reinterpret_cast<HICON>(LoadImageW(resources,
                    MAKEINTRESOURCEW(1), IMAGE_ICON, size, size, 0));
                const auto pixels = readIcon(icon);
                DestroyIcon(icon);
                require(size > 0 && pixels.width == size && pixels.height == size,
                        "The EXE icon cannot render at a common display scale");
            }
        }
        wchar_t temporary[MAX_PATH] = {};
        require(GetTempPathW(ARRAYSIZE(temporary), temporary) != 0, "Cannot locate TEMP");
        isolated = std::filesystem::absolute(temporary) /
            (L"tvi-" + std::to_wstring(GetCurrentProcessId()) + L"-" + std::to_wstring(GetTickCount64()));
        require(CreateDirectoryW(isolated.c_str(), nullptr) != FALSE,
                "Cannot create a new isolated test directory");
        const auto settings = isolated / L"TextViewer";
        std::filesystem::create_directory(settings);
        std::ofstream(settings / L"settings.json") << "{\"autoCheckUpdates\":false}";
        for (const auto name : {L"TEMP", L"TMP", L"APPDATA", L"XDG_RUNTIME_DIR"}) {
            require(SetEnvironmentVariableW(name, isolated.c_str()) != FALSE,
                    "Cannot isolate child preferences or single-instance socket");
        }
        const std::wstring name = L"TextViewerIconTest-" + std::to_wstring(GetCurrentProcessId());
        desktop = CreateDesktopW(name.c_str(), nullptr, nullptr, 0, GENERIC_ALL, nullptr);
        require(desktop != nullptr, "Cannot create a private desktop");
        const auto exe = std::filesystem::absolute(argv[1]).wstring();
        verifyLaunch(exe, name, desktop, isolated / L"sample.txt", resources);
        verifyLaunch(exe, name, desktop, isolated / L"sample.textviewer-icon-test", resources);
        CloseDesktop(desktop);
        desktop = nullptr;
        // Nonrecursive cleanup preserves any unexpected files.
        DeleteFileW((isolated / L"sample.txt").c_str());
        DeleteFileW((isolated / L"sample.textviewer-icon-test").c_str());
        DeleteFileW((settings / L"settings.json").c_str());
        RemoveDirectoryW(settings.c_str());
        RemoveDirectoryW(isolated.c_str());
        FreeLibrary(resources);
        resources = nullptr;
        std::cout << "Native window pixels and 100%-300% EXE icon resource tests passed.\n";
        return 0;
    } catch (const std::exception &error) {
        if (desktop) CloseDesktop(desktop);
        if (resources) FreeLibrary(resources);
        std::cerr << error.what() << "\n";
        return 1;
    }
}
