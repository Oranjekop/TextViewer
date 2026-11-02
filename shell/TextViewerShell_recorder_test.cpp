#define WIN32_LEAN_AND_MEAN
#include <windows.h>
#include <shellapi.h>
#include <string>

int WINAPI wWinMain(HINSTANCE, HINSTANCE, PWSTR, int) {
    int argc = 0;
    auto **argv = CommandLineToArgvW(GetCommandLineW(), &argc);
    wchar_t directory[32768];
    const DWORD length = GetEnvironmentVariableW(L"TEXTVIEWER_SHELL_TEST_CAPTURE", directory, ARRAYSIZE(directory));
    if (!argv || argc < 3 || wcscmp(argv[1], L"--") != 0 || !length || length >= ARRAYSIZE(directory)) return 1;
    std::wstring content;
    for (int i = 2; i < argc; ++i) content += std::wstring(argv[i]) + L"\n";
    LocalFree(argv);
    const auto prefix = std::wstring(directory) + L"\\" + std::to_wstring(GetCurrentProcessId());
    HANDLE output = CreateFileW((prefix + L".tmp").c_str(), GENERIC_WRITE, 0, nullptr, CREATE_NEW, FILE_ATTRIBUTE_NORMAL, nullptr);
    if (output == INVALID_HANDLE_VALUE) return 2;
    DWORD written = 0;
    const DWORD bytes = static_cast<DWORD>(content.size() * sizeof(wchar_t));
    const bool ok = WriteFile(output, content.data(), bytes, &written, nullptr) && written == bytes;
    CloseHandle(output);
    if (!ok || !MoveFileW((prefix + L".tmp").c_str(), (prefix + L".args").c_str())) return 3;
    return 0;
}
