# Readability conventions

Organize files for a person encountering the project for the first time.
Names and paths should help them predict where something belongs, understand
its purpose, and follow related work without reconstructing development history.
Apply these principles as the project evolves; they do not prescribe a fixed
folder tree.

## Choose names that explain purpose

- Use concrete domain terms and consistent vocabulary across code, tests,
  configuration, and documentation. Avoid unexplained abbreviations and vague
  catchalls such as `misc`, `stuff`, or `utils`.
- Let each part of a path add useful context. Avoid repeating the entire parent
  folder name in every child, but keep filenames distinguishable in search
  results and editor tabs.
- Follow the language or tool's naming conventions. Within each kind of file,
  keep casing, word separators, and suffixes consistent. Preserve conventional
  entrypoint names such as `README.md`.
- Name tests by behavior and fixtures by scenario. Use a consistent ordering
  when names include several dimensions, such as component and environment.
- Avoid names such as `new`, `final`, `copy`, and `v2` as substitutes for clear
  purpose. Use version labels only when versions are meaningful to readers;
  let version control preserve editing history.

## Group by responsibility and relationship

Keep files that explain or implement the same concept close together. Choose
boundaries that reflect a recognizable feature, responsibility, or dependency
boundary. A reader should be able to explain why a file belongs in its folder.

Prefer the simplest structure that makes navigation clear. Add a folder when
it gives related material a useful name or establishes a real boundary. Split
crowded folders by coherent responsibilities; avoid both deep chains of
single-child folders and sprawling directories of unrelated files. There is
no universal file-count or nesting-depth target.

Keep the organizing principle consistent among sibling folders. Avoid mixing
feature names, technical layers, and workflow stages at the same level without
a clear reason. Do not introduce speculative layers or a new package for every
file. Respect the language's visibility and dependency rules.

Place tests, fixtures, examples, and documentation close to what they support,
subject to ecosystem conventions. Give shared material a clear owner and move
it to a shared location when multiple consumers actually need it. Avoid turning
shared folders into a dumping ground.

## Keep files focused and artifacts tidy

Give each file a coherent purpose. Split files when distinct responsibilities
make them difficult to scan, and combine fragments when readers must constantly
jump between them to understand one operation. Use conceptual boundaries rather
than arbitrary line limits.

Separate generated output and temporary experiments from maintained source.
Use predictable output locations and ignore disposable artifacts in version
control. Retained fixtures, diagrams, and reports should have descriptive names
and enough context to explain their purpose, origin, and how to regenerate them
when applicable. Do not leave unexplained scratch files or competing copies of
the same document beside the maintained version.

## Provide a clear starting point

Give substantial components or collections a short README that explains their
purpose, important relationships, and where to start reading. Include runnable
commands with prerequisites and working-directory assumptions when relevant.
Link the key files and supporting material instead of maintaining an exhaustive
directory listing.

Use descriptive document titles and link text. Keep current instructions,
future plans, and historical notes clearly distinguished. Maintain one
authoritative explanation for each topic and link to it from other entrypoints.
Add local navigation where it helps readers; a README in every trivial folder
usually adds maintenance without helping discovery.

## Maintain navigation as the structure changes

When adding material, check whether a reader can find it from the relevant
entrypoint and predict its purpose from its name and location. When moving or
renaming files, update imports, scripts, configuration, templates, commands,
links, and indexes in the same change. Search for old references and run the
checks affected by the move.

Improve organization within the scope of the work. Make deliberate structural
changes when they reduce the effort needed to understand the project, and avoid
unrelated churn merely to enforce cosmetic uniformity.
