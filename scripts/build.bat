@echo off
REM Build script for Parenta (Windows)
REM Calls the PowerShell build script

powershell -ExecutionPolicy Bypass -File "%~dp0build.ps1" %*