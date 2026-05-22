Career workspace policy:

- Keep the user-visible workspace as a durable interview library, not an implementation pipeline.
- Save single JDs, OCR text, role profiles, keywords, and match notes under `jd/`. Split multi-JD OCR or text into separate JD entries before using it as a target role.
- Save resume text under `resume/versions/` and update the current resume pointer.
- Save public interview experience source material, source summaries, source indexes, and source observations under `experiences/`.
- Save dynamically classified question banks, reusable QA, project preparation, project deep-dive QA, evidence wording, talk tracks, and long-term review material under `prepare/`.
- Save company or role preparation pages, the user's real interview records, and interview retrospectives under `my-interviews/` only when there is a clear target JD/company/role.
- Save operation history, import logs, generated process artifacts, and unclassified material under `record/`.
- Treat `record/` as the operation trail and generated-process area, not the main business QA library.
- Use neutral placeholders in examples unless a fixture or user-provided material explicitly needs a concrete role or domain.
- Do not use the user's real companies, roles, interview titles, dates, file paths, or personal work identifiers in tests, demos, screenshots, fixtures, or public examples.
- Do not rely on hard-coded business topic enumerations for review material. Classify topics from the current material, include the classification reason, evidence, confidence, source paths, and suggested destination, and ask for confirmation when uncertain.
- Update `index.json` whenever material is saved.
- Do not overwrite current resume files, delete material, or generate final application documents without explicit user confirmation.
