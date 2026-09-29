import asyncio, sys
from playwright.async_api import async_playwright
import os
HERE=os.path.dirname(os.path.abspath(__file__))  # 测试用的论文文件放在本目录
B=sys.argv[1] if len(sys.argv)>1 else "http://127.0.0.1:18817"
SP=HERE; OUT=os.path.join(HERE,"out","jobs")
async def api(pg, method, path, body=None):
    return await pg.evaluate("""async ([m,p,b])=>{const r=await fetch(p,{method:m,headers:{'Content-Type':'application/json','X-KY':'1'},body:b?JSON.stringify(b):undefined});return [r.status, await r.json().catch(()=>null)]}""",[method,path,body])
def one(s): return s.replace("\n"," | ")
async def main():
    os.makedirs(OUT, exist_ok=True)
    async with async_playwright() as p:
        b=await p.chromium.launch(); errs=[]
        ctx=await b.new_context(viewport={"width":1360,"height":900})
        pg=await ctx.new_page(); pg.on("pageerror", lambda e: errs.append(str(e)))
        pg.on("response", lambda r: errs.append(f"{r.status} {r.url}") if r.status>=400 else None)
        await pg.goto(B)
        await api(pg,"POST","/api/setup",{"OrgName":"测试组","Username":"admin","Name":"陈老师","Password":"teach123"})
        await api(pg,"POST","/api/login",{"username":"admin","password":"teach123"})
        st,m=await api(pg,"POST","/api/models",{"name":"测试","base_url":"http://127.0.0.1:11434/v1","model":"qwen","key":"k"})
        mid=m["mine"][0]["id"]; await api(pg,"POST",f"/api/models/{mid}/check"); await api(pg,"POST",f"/api/models/{mid}/activate",{"active":True})
        await pg.goto(B+"/#/writing/draft"); await pg.reload(); await pg.wait_for_selector("form[data-submit=dfPlan]")
        async def start(idea, sec):
            await pg.select_option("#dfSection", sec)
            await pg.fill("form[data-submit=dfPlan] textarea[name=idea]", idea)
            await pg.click("form[data-submit=dfPlan] button[type=submit]")
        await start("短视频使用时间越长，大学生注意力越难集中，但原因不清楚。\n我们设计了两周的无推荐流干预。", "引言")
        await pg.wait_for_timeout(400)
        await start("本研究的主要结论是频繁切换内容会降低持续注意力，干预有效。\n下一步会扩大样本。", "结论")
        await pg.wait_for_timeout(800)
        print("STATE:", await pg.inner_text("#dfState"))
        print("LIST (running):", one(await pg.inner_text("#dfList"))[:300])
        await pg.wait_for_selector("#jobsBtn:not(.hide)")
        print("JOBS BTN:", await pg.inner_text("#jobsBtn"))
        await pg.screenshot(path=f"{OUT}/running.png")
        # 离开页面
        await pg.goto(B+"/#/"); await pg.wait_for_timeout(300)
        await pg.wait_for_selector("#toast:not(.hide):has-text('已完成')", timeout=20000)
        print("TOAST:", await pg.inner_text("#toast"))
        await pg.wait_for_timeout(4000)
        print("JOBS BTN after:", await pg.inner_text("#jobsBtn"))
        await pg.click("#jobsBtn"); await pg.wait_for_selector("#jobsModal .jobrow")
        print("CENTER:", one(await pg.inner_text("#jobsModal"))[:400])
        await pg.screenshot(path=f"{OUT}/center.png")
        await pg.click("#jobsModal button[data-act=jobOpen] >> nth=0")
        await pg.wait_for_selector("#dfClaims .dfclaim", timeout=10000)
        print("OPENED:", one(await pg.inner_text("#dfPlanBox"))[:120])
        print("LIST:", one(await pg.inner_text("#dfList"))[:300])
        # 起草也在后台：点起草后马上离开再回来
        await pg.click("button[data-act=dfWrite]"); await pg.wait_for_timeout(200)
        await pg.goto(B+"/#/mine"); await pg.wait_for_timeout(1500)
        await pg.goto(B+"/#/writing/draft"); await pg.wait_for_selector("#dfDraftBox .dftext", timeout=15000)
        print("DRAFT BACK:", one(await pg.inner_text("#dfDraftBox"))[:150])
        # 刷新整个网页后，后台任务仍然可见
        await pg.reload(); await pg.wait_for_selector("#jobsBtn:not(.hide)")
        print("AFTER RELOAD:", await pg.inner_text("#jobsBtn"))
        print("ERRORS:", errs); await b.close()
asyncio.run(main())
