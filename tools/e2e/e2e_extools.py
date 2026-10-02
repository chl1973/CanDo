# 外部工具服务：在设置里接入 Python 示例服务 → 智能体调用 → 确认调用 → 确认保存；设置里改回“每次询问”
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
        await pg.goto(B)
        await api(pg,"POST","/api/setup",{"org_name":"智能感知课题组","username":"admin","name":"陈老师","password":"teach123"})
        await api(pg,"POST","/api/login",{"username":"admin","password":"teach123"})
        st,m=await api(pg,"POST","/api/models",{"name":"DeepSeek","base_url":"http://127.0.0.1:11434/v1","model":"deepseek-v4-pro","key":"k"})
        mid=m["mine"][0]["id"]; await api(pg,"POST",f"/api/models/{mid}/check"); await api(pg,"POST",f"/api/models/{mid}/activate",{"active":True})
        await api(pg,"PUT","/api/agent/folders",{"folders":[{"path":WS,"write":True}]})
        await pg.goto(B+"/#/assistant/agent"); await pg.reload(); await pg.wait_for_selector("#agText")
        # 在设置里接入
        await pg.click("button[data-act=agSide][data-s=folders]"); await pg.wait_for_selector("form[data-submit=agExtAdd]")
        await pg.fill("form[data-submit=agExtAdd] input[name=name]","fig"); await pg.fill("form[data-submit=agExtAdd] input[name=url]",EXT)
        await pg.click("form[data-submit=agExtAdd] button[type=submit]"); await pg.wait_for_selector(".extsvc .tag.ok")
        print("EXT SECTION:", one(await pg.inner_text(".extsvc")))
        await pg.locator(".extsvc").screenshot(path=f"{OUT}/settings.png")
        # 智能体调用 → 确认调用
        await pg.fill("#agText","把 3、5、2 画成柱状图"); await pg.click("#agGo")
        await pg.wait_for_selector("button[data-act=agApprove][data-d=always]", timeout=30000)
        print("APPROVAL:", one(await pg.inner_text("#agPending") if await pg.query_selector("#agPending") else await pg.inner_text("#agSteps"))[:300])
        await pg.screenshot(path=f"{OUT}/approval.png")
        await pg.click("button[data-act=agApprove][data-d=always]")
        await pg.wait_for_selector("#agSideBody .change", timeout=30000)
        await pg.wait_for_function("() => !document.querySelector('#agGo').disabled", timeout=30000)
        print("STEPS:", one(await pg.inner_text("#agSteps"))[-300:])
        chs=await pg.locator("#agSideBody .change").all_inner_texts(); print("CHANGES:", [one(c)[:120] for c in chs])
        print("BEFORE APPLY exists:", os.path.exists(os.path.join(WS,"chart.svg")))
        await pg.locator("#agSideBody .change button[data-a=apply]").first.click(); await pg.wait_for_timeout(800)
        print("AFTER APPLY exists:", os.path.exists(os.path.join(WS,"chart.svg")))
        await pg.screenshot(path=f"{OUT}/applied.png")
        # 设置里能看到“始终允许”，并能改回
        await pg.click("button[data-act=agSide][data-s=folders]"); await pg.wait_for_selector("button[data-act=agExtRevoke]")
        await pg.locator(".extsvc").screenshot(path=f"{OUT}/settings_always.png")
        await pg.click("button[data-act=agExtRevoke]"); await pg.wait_for_timeout(500)
        st,v=await api(pg,"GET","/api/agent/extools")
        print("ALLOW AFTER REVOKE:", v["services"][0]["allow"])
        print("ERRORS:", errs); await b.close()
asyncio.run(main())
