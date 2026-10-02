# 能力中心 + 外部工具服务：空状态 → 接入 Python 示例服务 → 工具卡片 → “用它”带示例跳到智能体 → 确认调用 → 确认保存
# → 能力中心里改回“每次询问” → 下载开发包；另外截深色和手机的图
import asyncio, sys, os, tempfile
from playwright.async_api import async_playwright
HERE=os.path.dirname(os.path.abspath(__file__))
B=sys.argv[1] if len(sys.argv)>1 else "http://127.0.0.1:18826"
EXT=sys.argv[2] if len(sys.argv)>2 else "http://127.0.0.1:18765"
OUT=os.path.join(HERE,"out","extools")
def one(s): return s.replace("\n"," | ")
async def api(pg, method, path, body=None):
    return await pg.evaluate("""async ([m,p,b])=>{const r=await fetch(p,{method:m,headers:{'Content-Type':'application/json','X-KY':'1'},body:b?JSON.stringify(b):undefined});return [r.status, await r.json().catch(()=>null)]}""",[method,path,body])
async def main():
    os.makedirs(OUT, exist_ok=True)
    WS=tempfile.mkdtemp(prefix="cando-ext-")
    async with async_playwright() as p:
        b=await p.chromium.launch(); errs=[]
        ctx=await b.new_context(viewport={"width":1360,"height":900})
        pg=await ctx.new_page(); pg.on("pageerror", lambda e: errs.append(str(e)))
        pg.on("response", lambda r: errs.append(f"{r.status} {r.url}") if r.status>=500 else None)
        pg.on("dialog", lambda d: asyncio.ensure_future(d.accept()))
        await pg.goto(B)
        await api(pg,"POST","/api/setup",{"org_name":"智能感知课题组","username":"admin","name":"陈老师","password":"teach123"})
        await api(pg,"POST","/api/login",{"username":"admin","password":"teach123"})
        st,m=await api(pg,"POST","/api/models",{"name":"DeepSeek","base_url":"http://127.0.0.1:11434/v1","model":"deepseek-v4-pro","key":"k"})
        mid=m["mine"][0]["id"]; await api(pg,"POST",f"/api/models/{mid}/check"); await api(pg,"POST",f"/api/models/{mid}/activate",{"active":True})
        # 还没授权文件夹：文件类能力应标“需要设置”
        await pg.goto(B+"/#/assistant/tools"); await pg.reload(); await pg.wait_for_selector(".cchero")
        print("TABS:", one(await pg.inner_text(".tabs")))
        print("STATS (no folder):", one(await pg.inner_text(".ccstats")), "| need-setup cards:", await pg.locator(".cccard.off").count())
        print("EMPTY:", one(await pg.inner_text(".ccempty"))[:60])
        await pg.screenshot(path=f"{OUT}/center_empty.png", full_page=True)
        # “去设置”跳到智能体的“文件夹与权限”
        await pg.locator(".cccard.off a[data-act=ccGo]").first.click(); await pg.wait_for_selector("form[data-submit=agAddFolder]")
        print("GO SETUP -> folders tab:", await pg.locator(".tabs.sub button.on").inner_text(), "| pointer:", "能力中心" in await pg.inner_text("#agSideBody"))
        await api(pg,"PUT","/api/agent/folders",{"folders":[{"path":WS,"write":True}]})
        await pg.click("#agSideBody a[data-act=asGoTools]"); await pg.wait_for_selector(".cchero")
        # 接入
        await pg.click("button[data-act=ccAdd]"); await pg.wait_for_selector("form[data-submit=ccAddGo]")
        await pg.fill("form[data-submit=ccAddGo] input[name=name]","fig"); await pg.fill("form[data-submit=ccAddGo] input[name=url]",EXT)
        await pg.click("form[data-submit=ccAddGo] button[type=submit]"); await pg.wait_for_selector(".ccsvc .tag.ok")
        print("SERVICE:", one(await pg.inner_text(".ccsvchead"))[:120])
        cards=await pg.locator(".cccard.ext").all_inner_texts(); print("EXT CARDS:", len(cards), "|", one(cards[0])[:200])
        print("STATS:", one(await pg.inner_text(".ccstats")))
        await pg.wait_for_timeout(300); await pg.screenshot(path=f"{OUT}/center.png", full_page=True)
        # 搜索
        await pg.fill("#ccFind","word"); await pg.wait_for_timeout(200)
        print("SEARCH word -> visible cards:", await pg.locator(".cccard:not(.hide)").count(), "| visible groups:", await pg.locator(".ccgroup:not(.hide)").count(), "| ext service hidden:", await pg.locator(".ccsvc.hide").count())
        await pg.fill("#ccFind","柱状图"); await pg.wait_for_timeout(200)
        print("SEARCH 柱状图 -> visible cards:", await pg.locator(".cccard:not(.hide)").count())
        await pg.fill("#ccFind",""); await pg.wait_for_timeout(200)
        # 用它 → 智能体，示例已填好
        await pg.locator(".cccard.ext button[data-act=ccUse]").first.click(); await pg.wait_for_selector("#agText")
        print("USE -> draft:", await pg.input_value("#agText"))
        await pg.wait_for_selector("#agTools .chip"); print("TOOL STRIP:", one(await pg.inner_text("#agTools")))
        # 智能体调用 → 确认调用
        await pg.click("#agGo")
        await pg.wait_for_selector("button[data-act=agApprove][data-d=always]", timeout=30000)
        print("APPROVAL:", one(await pg.inner_text("#agSteps"))[:200])
        await pg.screenshot(path=f"{OUT}/approval.png")
        await pg.click("button[data-act=agApprove][data-d=always]")
        await pg.wait_for_selector("#agSideBody .change", timeout=30000)
        await pg.wait_for_function("() => !document.querySelector('#agGo').disabled", timeout=30000)
        chs=await pg.locator("#agSideBody .change").all_inner_texts(); print("CHANGES:", [one(c)[:120] for c in chs])
        print("BEFORE APPLY exists:", os.path.exists(os.path.join(WS,"chart.svg")))
        await pg.locator("#agSideBody .change button[data-a=apply]").first.click(); await pg.wait_for_timeout(800)
        print("AFTER APPLY exists:", os.path.exists(os.path.join(WS,"chart.svg")))
        await pg.screenshot(path=f"{OUT}/applied.png")
        # 能力中心里能看到“始终允许”，并能改回
        await pg.click("#agTools a[data-act=asGoTools]"); await pg.wait_for_selector("button[data-act=ccRevoke]")
        print("ALWAYS shown:", "已设为始终允许" in await pg.inner_text(".cccard.ext >> nth=0"))
        await pg.click("button[data-act=ccRevoke]"); await pg.wait_for_function("() => !document.querySelector('button[data-act=ccRevoke]')")
        st,v=await api(pg,"GET","/api/agent/extools")
        print("ALLOW AFTER REVOKE:", v["services"][0]["allow"])
        # 停用 → 卡片变灰、不能用
        await pg.select_option("select[data-change=ccToggle]","0"); await pg.wait_for_selector(".cccard.ext.off")
        print("DISABLED: use buttons disabled:", await pg.locator(".cccard.ext button[data-act=ccUse][disabled]").count(), "| stat:", one(await pg.inner_text(".ccstat.ext")))
        await pg.select_option("select[data-change=ccToggle]","1"); await pg.wait_for_selector(".cccard.ext:not(.off)")
        # 开发包
        kit=await pg.evaluate("async ()=>{const r=await fetch('/api/agent/extools/kit');const b=await r.arrayBuffer();return [r.status,r.headers.get('content-type'),b.byteLength]}")
        print("KIT:", kit[0], kit[1], kit[2]>5000)
        # 深色、手机
        await pg.emulate_media(color_scheme="dark"); await pg.wait_for_timeout(300); await pg.screenshot(path=f"{OUT}/center_dark.png")
        mc=await b.new_context(viewport={"width":390,"height":844},device_scale_factor=2, storage_state=await ctx.storage_state()); mp=await mc.new_page(); mp.on("pageerror", lambda e: errs.append("M:"+str(e)))
        await mp.goto(B+"/#/assistant/tools"); await mp.wait_for_selector(".cccard.ext"); await mp.wait_for_timeout(400)
        await mp.screenshot(path=f"{OUT}/m_center.png")
        print("MOBILE overflow:", await mp.evaluate("document.documentElement.scrollWidth > window.innerWidth"))
        print("ERRORS:", errs); await b.close()
asyncio.run(main())
