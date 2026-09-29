import asyncio, sys, os
from playwright.async_api import async_playwright
B=os.path.dirname(os.path.abspath(__file__))
JOBS=[("cando-icon.svg",[512,192,180,144,96,72,64,48]),("cando-icon-small.svg",[64,32,24,16]),("cando-icon-fullbleed.svg",[1024,180]),("cando-icon.svg",[256,128,1024])]
async def main():
    async with async_playwright() as p:
        b=await p.chromium.launch()
        for f,sizes in JOBS:
            for s in sizes:
                pg=await b.new_page(viewport={"width":s,"height":s})
                await pg.goto(f"file://{B}/{f}")
                await pg.wait_for_timeout(100)
                await pg.screenshot(path=f"{B}/png/{f[:-4]}-{s}.png", omit_background=True)
                await pg.close()
        await b.close()
os.makedirs(B+"/png",exist_ok=True); asyncio.run(main())
