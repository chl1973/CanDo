import asyncio, sys, datetime
from playwright.async_api import async_playwright
import os
HERE=os.path.dirname(os.path.abspath(__file__))  # 测试用的论文文件放在本目录
B=sys.argv[1] if len(sys.argv)>1 else "http://127.0.0.1:18820"
SP=HERE; OUT=os.path.join(HERE,"out","main")
def one(s): return s.replace("\n"," | ")
async def api(pg, method, path, body=None):
    return await pg.evaluate("""async ([m,p,b])=>{const r=await fetch(p,{method:m,headers:{'Content-Type':'application/json','X-KY':'1'},body:b?JSON.stringify(b):undefined});return [r.status, await r.json().catch(()=>null)]}""",[method,path,body])
async def main():
    os.makedirs(OUT, exist_ok=True)

    async with async_playwright() as p:
        b=await p.chromium.launch(); errs=[]
        ctx=await b.new_context(viewport={"width":1360,"height":900})
        pg=await ctx.new_page(); pg.on("pageerror", lambda e: errs.append(str(e)))
        pg.on("response", lambda r: errs.append(f"{r.status} {r.url}") if r.status>=400 else None)
        # 1 首次设置页（品牌页）→ 通过表单完成
        await pg.goto(B); await pg.wait_for_selector(".authside")
        await pg.fill("input[name=org_name]","智能感知课题组"); await pg.fill("input[name=name]","陈老师"); await pg.fill("input[name=username]","admin")
        await pg.fill("input[name=password]","teach123"); await pg.fill("input[name=password2]","teach123"); await pg.click("form[data-submit=doSetup] button")
        await pg.wait_for_selector(".kpis"); print("SETUP->HOME ok; guest removed:", await pg.evaluate("!document.body.classList.contains('guest')"))
        st,m=await api(pg,"POST","/api/models",{"name":"DeepSeek","base_url":"http://127.0.0.1:11434/v1","model":"deepseek-v4-pro","key":"k"})
        mid=m["mine"][0]["id"]; await api(pg,"POST",f"/api/models/{mid}/check"); await api(pg,"POST",f"/api/models/{mid}/activate",{"active":True})
        st,u=await api(pg,"POST","/api/users",{"username":"xiaoming","name":"李小明","password":"stud123"})
        st,pj=await api(pg,"POST","/api/projects",{"name":"短视频对大学生注意力的影响","template_key":"research","members":[u["id"]]})
        pid=pj["id"]; st,pr=await api(pg,"GET",f"/api/projects/{pid}"); sids=[s["id"] for s in pr["stages"]]
        await api(pg,"PATCH",f"/api/projects/{pid}/stages/{sids[2]}",{"due":(datetime.date.today()+datetime.timedelta(days=2)).isoformat()})
        # 2 学生：提交两个阶段，老师退回一个
        sc=await b.new_context(viewport={"width":1360,"height":900}); sp=await sc.new_page(); sp.on("pageerror", lambda e: errs.append("S:"+str(e)))
        await sp.goto(B); await sp.wait_for_selector("form[data-submit=doLogin]"); await sp.screenshot(path=f"{OUT}/login.png")
        await sp.fill("input[name=username]","xiaoming"); await sp.fill("input[name=password]","stud123"); await sp.click("form[data-submit=doLogin] button"); await sp.wait_for_selector(".kpis")
        for s in sids[:2]: await api(sp,"POST",f"/api/projects/{pid}/stages/{s}/submit",{"content":"完成了","request_review":True})
        await api(pg,"POST",f"/api/projects/{pid}/stages/{sids[1]}/review",{"decision":"return","comment":"再补充几篇文献"})
        # 3 论文库：直接选文件即上传（多文件）、筛选、菜单共享
        await pg.goto(B+"/#/mine"); await pg.wait_for_selector("#upZone")
        await pg.set_input_files("#upZone input[type=file]", [SP+"/paper_a.md", SP+"/paper.pdf"])
        await pg.wait_for_selector("#toast:has-text('已上传 2 篇')", timeout=30000)
        print("LIB:", one(await pg.inner_text("#matList"))[:200], "|", await pg.inner_text("#libCount"))
        await pg.fill("#libQ","paper_a"); await pg.wait_for_timeout(150); print("FILTER:", await pg.inner_text("#libCount"))
        await pg.fill("#libQ",""); await pg.wait_for_timeout(150)
        await pg.click("button[data-act=matMenu] >> nth=0"); await pg.wait_for_selector(".menu.pop")
        await pg.screenshot(path=f"{OUT}/lib_menu.png")
        pg.once("dialog", lambda d: asyncio.ensure_future(d.accept()))
        await pg.click(".menu.pop button[data-act=shareMat]"); await pg.wait_for_selector(".tag.pri:has-text('已共享')")
        print("MENU closed:", await pg.locator(".menu.pop").count()==0)
        await pg.click("button[data-act=plSub][data-s=group]"); await pg.wait_for_selector("#matList table"); print("GROUP:", one(await pg.inner_text("#matList"))[:120])
        await pg.click("button[data-act=plSub][data-s=list]"); await pg.wait_for_selector("#matList table")
        # 扫描件：上传后提示 OCR（取消）
        dl=[]; pg.once("dialog", lambda d: (dl.append(d.message), asyncio.ensure_future(d.dismiss())))
        await pg.set_input_files("#upZone input[type=file]", SP+"/scan.pdf"); await pg.wait_for_selector("button[data-act=ocrMat]", timeout=30000)
        print("OCR prompt:", one(dl[0])[:60] if dl else "none")
        await pg.select_option("#libF","ocr"); await pg.wait_for_timeout(150); print("OCR filter:", await pg.inner_text("#libCount"))
        await pg.select_option("#libF",""); 
        # 删除（菜单）
        n0=await pg.locator("#matList tbody tr").count()
        pg.on("dialog", lambda d: asyncio.ensure_future(d.accept()))
        await pg.click("button[data-act=matMenu] >> nth=0"); await pg.click(".menu.pop button[data-act=delMat]"); await pg.wait_for_timeout(800)
        print("DELETE:", n0, "->", await pg.locator("#matList tbody tr").count())
        await pg.screenshot(path=f"{OUT}/lib.png")
        # 4 AI 起草，首页“最近的草稿”能打开
        await pg.goto(B+"/#/writing/draft"); await pg.wait_for_selector("form[data-submit=dfPlan]")
        await pg.fill("form[data-submit=dfPlan] textarea[name=idea]","短视频使用时间越长，大学生注意力越难集中，但原因不清楚。\n我们设计了两周的无推荐流干预。")
        await pg.click("form[data-submit=dfPlan] button[type=submit]"); await pg.wait_for_selector("#dfClaims .dfclaim", timeout=30000)
        # 5 首页总览
        await pg.goto(B+"/#/"); await pg.wait_for_selector(".kpis")
        print("KPI teacher:", one(await pg.inner_text(".kpis")))
        print("TODO teacher:", one(await pg.inner_text(".homegrid .card >> nth=0")))
        await pg.screenshot(path=f"{OUT}/home.png", full_page=True)
        await pg.click("a[data-act=homeDraft]"); await pg.wait_for_selector("#dfClaims .dfclaim", timeout=10000); print("HOME DRAFT opened:", one(await pg.inner_text("#dfPlanBox"))[:40])
        await pg.goto(B+"/#/"); await pg.wait_for_selector(".homegrid a.lrow[href*='/flow/']")
        await pg.click(".homegrid a.lrow[href*='/flow/']"); await pg.wait_for_selector(".step .dot.sel"); print("TODO -> stage:", await pg.inner_text(".step .dot.sel"))
        await sp.goto(B+"/#/"); await sp.reload(); await sp.wait_for_selector(".kpis")
        print("KPI student:", one(await sp.inner_text(".kpis")))
        print("TODO student:", one(await sp.inner_text(".homegrid .card >> nth=0")))
        await sp.screenshot(path=f"{OUT}/home_student.png")
        # 6 外观：切换并在刷新后保持
        await pg.goto(B+"/#/settings"); await pg.wait_for_selector(".themeseg")
        await pg.click(".themeseg button[data-t=dark]"); await pg.reload(); await pg.wait_for_selector(".themeseg button.on")
        print("THEME after reload:", await pg.evaluate("document.documentElement.dataset.theme"), await pg.inner_text(".themeseg button.on"))
        print("BG:", await pg.evaluate("getComputedStyle(document.body).backgroundColor"))
        await pg.goto(B+"/#/"); await pg.wait_for_selector(".kpis"); await pg.screenshot(path=f"{OUT}/home_dark.png", full_page=True)
        await pg.goto(B+f"/#/p/{pid}"); await pg.wait_for_timeout(900); await pg.screenshot(path=f"{OUT}/project_dark.png")
        await pg.goto(B+"/#/assistant/agent"); await pg.wait_for_timeout(900); await pg.screenshot(path=f"{OUT}/agent_dark.png")
        await pg.click("#themeBtn"); print("CYCLE ->", await pg.evaluate("document.documentElement.dataset.theme || 'auto'"))
        # Esc 关闭弹窗
        await pg.click("#jobsBtn"); await pg.wait_for_selector("#modal:not(.hide)"); await pg.keyboard.press("Escape"); print("ESC closes:", await pg.evaluate("document.querySelector('#modal').classList.contains('hide')"))
        # 7 手机：底部 5 个 + 更多
        mc=await b.new_context(viewport={"width":390,"height":844},device_scale_factor=2, storage_state=await sc.storage_state()); mp=await mc.new_page(); mp.on("pageerror", lambda e: errs.append("M:"+str(e)))
        await mp.goto(B+"/#/"); await mp.wait_for_selector(".kpis")
        vis=await mp.evaluate("[...document.querySelectorAll('#nav a')].filter(a=>a.offsetParent).map(a=>a.innerText.trim())"); print("MOBILE NAV:", vis)
        await mp.screenshot(path=f"{OUT}/m_home.png")
        await mp.click("#nav a[data-nav=more]"); await mp.wait_for_selector(".sheetgrid"); print("SHEET:", one(await mp.inner_text(".sheetgrid")))
        await mp.screenshot(path=f"{OUT}/m_more.png")
        await mp.click(".sheetgrid a[href='#/settings']"); await mp.wait_for_timeout(600)
        print("MORE active on settings:", await mp.evaluate("document.querySelector('#nav a[data-nav=more]').classList.contains('on')"))
        await mp.goto(B+"/#/mine"); await mp.wait_for_selector("#matList"); await mp.screenshot(path=f"{OUT}/m_mine.png")
        print("ERRORS:", [e for e in errs if "/api/models/" not in e]); await b.close()
asyncio.run(main())
