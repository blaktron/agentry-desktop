# Windows code signing: what is pinned here

`microsoft-identity-verification-root-2020.pem` is the root of the chain
every Azure Artifact Signing certificate — and its timestamp — chains to:
`CN=Microsoft Identity Verification Root Certificate Authority 2020`, SHA-256
fingerprint `53:67:F2:0C:7A:DE:0E:2B:CA:79:09:15:05:6D:08:6B:72:0C:33:C1:FA:2A:26:61:AC:F7:87:E3:29:2E:12:70`,
valid to 2045-04-16, published at
<https://www.microsoft.com/pkiops/docs/repository.htm>. It is committed
because it is not in the Mozilla bundle Ubuntu's `ca-certificates` ships,
so `osslsigncode verify` has to be handed it; `scripts/check-windows-installer.sh
--signed` refuses to use a file whose fingerprint differs from the one above.

The fingerprint is enforced in `scripts/check-windows-installer.sh`
(`pinned_fp`); `scripts/acceptance/windows-signing.sh` checks that this file
and that pin agree; the README above is the human copy. `scripts/sign-windows.sh`
pins the `jsign` release that produces the signature (version and SHA-256 of
the jar, in the script; the digest was computed from the jar GitHub serves
for that release tag). Changing any pin is a pull request,
like every other pin in this repository.
