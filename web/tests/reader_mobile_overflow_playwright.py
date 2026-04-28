import argparse
from urllib.parse import urljoin

from playwright.sync_api import Page, sync_playwright


VIEWPORT = {"width": 390, "height": 844}
ROUTES = ("/p/pindoc/today", "/p/pindoc/graph")


def page_url(base_url: str, route: str) -> str:
    return urljoin(base_url.rstrip("/") + "/", route.lstrip("/"))


def overflow_metrics(page: Page) -> dict:
    return page.evaluate(
        """() => {
          const doc = document.documentElement;
          const body = document.body;
          return {
            documentScrollWidth: doc.scrollWidth,
            documentClientWidth: doc.clientWidth,
            bodyScrollWidth: body.scrollWidth,
            bodyClientWidth: body.clientWidth,
          };
        }"""
    )


def assert_no_document_overflow(page: Page, route: str) -> None:
    metrics = overflow_metrics(page)
    assert metrics["documentScrollWidth"] <= metrics["documentClientWidth"] + 1, (
        f"{route} document overflow: "
        f"{metrics['documentScrollWidth']} > {metrics['documentClientWidth']}"
    )
    assert metrics["bodyScrollWidth"] <= metrics["bodyClientWidth"] + 1, (
        f"{route} body overflow: {metrics['bodyScrollWidth']} > {metrics['bodyClientWidth']}"
    )


def assert_today_cards_fit(page: Page) -> None:
    overflow = page.evaluate(
        """() => Array.from(document.querySelectorAll(
          '.today, .change-card, .change-card__top, .change-card h2'
        )).filter((el) => el.scrollWidth > el.clientWidth + 1).map((el) => ({
          tag: el.tagName.toLowerCase(),
          className: String(el.className),
          scrollWidth: el.scrollWidth,
          clientWidth: el.clientWidth,
          text: (el.textContent || '').trim().slice(0, 120),
        }))"""
    )
    assert overflow == [], f"Today card content overflowed: {overflow}"


def assert_graph_focus_fits(page: Page) -> None:
    result = page.evaluate(
        """() => {
          const wrap = document.querySelector('.graph-canvas-wrap')?.getBoundingClientRect();
          const focus = document.querySelector('.graph-canvas__node--focus')?.getBoundingClientRect();
          const focusTitle = document
            .querySelector('.graph-canvas__node--focus .graph-canvas__node-title')
            ?.getBoundingClientRect();
          const atlas = document.querySelector('.atlas-minimap')?.getBoundingClientRect();
          const within = (inner, outer) => Boolean(inner && outer)
            && inner.left >= outer.left - 1
            && inner.right <= outer.right + 1
            && inner.top >= outer.top - 1
            && inner.bottom <= outer.bottom + 1;
          return {
            focusWithinWrap: within(focus, wrap),
            focusTitleWithinWrap: within(focusTitle, wrap),
            atlasWithinWrap: within(atlas, wrap),
          };
        }"""
    )
    assert result == {
        "focusWithinWrap": True,
        "focusTitleWithinWrap": True,
        "atlasWithinWrap": True,
    }, f"Graph focus or atlas clipped at mobile width: {result}"


def run(base_url: str) -> None:
    with sync_playwright() as p:
        browser = p.chromium.launch()
        page = browser.new_page(viewport=VIEWPORT)
        page.emulate_media(reduced_motion="reduce")
        try:
            for route in ROUTES:
                page.goto(page_url(base_url, route), wait_until="domcontentloaded")
                if route.endswith("/today"):
                    page.wait_for_selector(".today", state="visible")
                    assert_today_cards_fit(page)
                else:
                    page.wait_for_selector(".graph-canvas-wrap", state="visible")
                    page.wait_for_selector(".graph-canvas__node--focus", state="visible")
                    assert_graph_focus_fits(page)
                assert_no_document_overflow(page, route)
        finally:
            browser.close()


def main() -> None:
    parser = argparse.ArgumentParser(
        description="Assert Reader Today and Graph have no 390px horizontal overflow."
    )
    parser.add_argument(
        "--base-url",
        default="http://127.0.0.1:5831",
        help="Reader web base URL. Run Vite on 5831 with the API server on 5830, or pass another URL.",
    )
    args = parser.parse_args()
    run(args.base_url)


if __name__ == "__main__":
    main()
