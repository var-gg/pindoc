import { renderToStaticMarkup } from "react-dom/server";
import type { AssetRef } from "../src/api/client";
import { I18nProvider } from "../src/i18n";
import { ArtifactAssets, groupVisibleAssetsByRole } from "../src/reader/ArtifactAssets";
import en from "../src/i18n/en.json";
import ko from "../src/i18n/ko.json";

const enCopy = en as Record<string, string>;
const koCopy = ko as Record<string, string>;

function assert(condition: boolean, message: string): void {
  if (!condition) throw new Error(message);
}

function assertEqual(actual: unknown, expected: unknown, message: string): void {
  if (actual !== expected) {
    throw new Error(`${message}: got ${String(actual)}, want ${String(expected)}`);
  }
}

function asset(
  role: AssetRef["role"],
  displayOrder: number,
  filename: string,
  id = `${role}-${displayOrder}`,
  crossVisibility?: AssetRef["cross_visibility"],
): AssetRef {
  const ref: AssetRef = {
    id,
    asset_ref: `pindoc-asset://${id}`,
    role,
    mime_type: role === "inline_image" ? "image/png" : "text/plain",
    size_bytes: role === "inline_image" ? 4096 : 199,
    original_filename: filename,
    blob_url: `/blob/${id}`,
    is_image: role === "inline_image",
    projection: {},
    display_order: displayOrder,
  };
  if (crossVisibility) ref.cross_visibility = crossVisibility;
  return ref;
}

function testAssetGroupsUseEvidencePriority(): void {
  const groups = groupVisibleAssetsByRole([
    asset("attachment", 0, "attachment.txt"),
    asset("inline_image", 1, "inline.png"),
    asset("generated_output", 2, "generated.txt"),
    asset("evidence", 3, "evidence.txt"),
  ]);

  assertEqual(
    groups.map((group) => group.role).join(","),
    "evidence,generated_output,attachment",
    "non-inline assets should be grouped by visual priority",
  );
  assertEqual(
    groups.flatMap((group) => group.assets).length,
    3,
    "inline_image assets should stay out of the attachment list",
  );
}

function testArtifactAssetsRenderGroupHeadersAndCards(): void {
  const sharedAssetId = "5494285d-c51d-41d4-9779-d8ec0fae2546";
  const html = renderToStaticMarkup(
    <I18nProvider projectLang="en">
      <ArtifactAssets
        assets={[
          asset("attachment", 0, "qa-attachment-probe.txt", sharedAssetId),
          asset("generated_output", 1, "qa-attachment-probe.txt", sharedAssetId),
          asset("evidence", 2, "qa-attachment-probe.txt", sharedAssetId),
          asset("inline_image", 3, "inline.png"),
        ]}
      />
    </I18nProvider>,
  );

  const evidenceIndex = html.indexOf("Evidence <span>· 1</span>");
  const generatedIndex = html.indexOf("Generated outputs <span>· 1</span>");
  const attachmentIndex = html.indexOf("Attachments <span>· 1</span>");
  assert(evidenceIndex >= 0, "evidence group header should render");
  assert(generatedIndex > evidenceIndex, "generated outputs group should follow evidence");
  assert(attachmentIndex > generatedIndex, "attachments group should follow generated outputs");
  assertEqual(
    html.split("qa-attachment-probe.txt").length - 1,
    3,
    "same fixture asset should render once in each non-inline role group",
  );
  assert(!html.includes("inline.png"), "inline image asset should not render in the attachment list");
  assert(!html.includes("Evidence · text/plain"), "role label should not be duplicated in card metadata");
}

function testArtifactAssetsRenderCrossVisibilityWarning(): void {
  const html = renderToStaticMarkup(
    <I18nProvider projectLang="en">
      <ArtifactAssets
        assets={[
          asset("attachment", 0, "shared.txt", "shared-asset", ["private", "public"]),
        ]}
      />
    </I18nProvider>,
  );

  assert(
    html.includes("Shared with public artifacts, private artifacts"),
    "cross-visibility warning badge should render with stable visibility order",
  );
  assert(!html.includes("reader.asset_cross_visibility"), "cross-visibility keys should not fall through");
}

function testArtifactAssetsRenderInlineImageWarningBanner(): void {
  const html = renderToStaticMarkup(
    <I18nProvider projectLang="en">
      <ArtifactAssets assets={[asset("inline_image", 0, "inline.png", "inline-asset", ["private"])]} />
    </I18nProvider>,
  );

  assert(
    html.includes("Inline images are also attached to private artifacts."),
    "inline-only cross-visibility warning should render an artifact-level banner",
  );
  assert(!html.includes("inline.png"), "inline-only warning should not add inline images to attachment cards");
}

function testAssetGroupI18nKeysExist(): void {
  const keys = [
    "reader.asset_group_attachment",
    "reader.asset_group_evidence",
    "reader.asset_group_generated_output",
    "reader.asset_cross_visibility.public",
    "reader.asset_cross_visibility.org",
    "reader.asset_cross_visibility.private",
    "reader.asset_cross_visibility_badge",
    "reader.asset_cross_visibility_inline_banner",
  ];

  for (const key of keys) {
    assert(typeof enCopy[key] === "string" && enCopy[key].length > 0, `missing EN i18n key: ${key}`);
    assert(typeof koCopy[key] === "string" && koCopy[key].length > 0, `missing KO i18n key: ${key}`);
    assert(enCopy[key] !== key, `EN i18n key should not fall through: ${key}`);
    assert(koCopy[key] !== key, `KO i18n key should not fall through: ${key}`);
  }
}

testAssetGroupsUseEvidencePriority();
testArtifactAssetsRenderGroupHeadersAndCards();
testArtifactAssetsRenderCrossVisibilityWarning();
testArtifactAssetsRenderInlineImageWarningBanner();
testAssetGroupI18nKeysExist();
