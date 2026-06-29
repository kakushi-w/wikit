# Third-party notices

wikit is licensed under the MIT License (see [LICENSE](LICENSE)). It is
compatible with the WikiComma archive format and bundles / depends on the
following third-party components.

## WikiComma (format compatibility)

wikit reads and writes the same on-disk archive format as WikiComma. The
original WikiComma project's MIT license (Copyright (c) 2022 DBotThePony) is
preserved in [LICENSE-WikiComma](LICENSE-WikiComma).

## 7-Zip (embedded binary)

wikit embeds the 7-Zip console binary for each platform to create `.7z`
archives. 7-Zip is Copyright (C) 1999-2026 Igor Pavlov and is distributed under
the GNU LGPL (with an unRAR license restriction on some code) and BSD licenses.
The full 7-Zip license is included at
[licenses/7-Zip-LICENSE.txt](licenses/7-Zip-LICENSE.txt), as required for
binary redistribution.

## Go modules

- **github.com/PuerkitoBio/goquery** — BSD-3-Clause
- **github.com/andybalholm/cascadia** — BSD-2-Clause
- **golang.org/x/net** — BSD-3-Clause

Their license texts are available in the Go module cache and in each project's
repository.
