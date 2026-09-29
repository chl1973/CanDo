package com.keyan.workbench;

// CanDo 安卓 App：连接老师电脑上运行的“CanDo 可为”，在 App 内使用全部功能。
// 数据全部保存在老师电脑上，手机只保存“电脑地址”和登录状态。

import android.Manifest;
import android.app.Activity;
import android.app.DownloadManager;
import android.content.Context;
import android.content.Intent;
import android.content.SharedPreferences;
import android.content.pm.PackageManager;
import android.graphics.Color;
import android.graphics.Typeface;
import android.net.Uri;
import android.os.Build;
import android.os.Bundle;
import android.os.Environment;
import android.text.InputType;
import android.view.Gravity;
import android.view.KeyEvent;
import android.view.View;
import android.view.ViewGroup;
import android.view.inputmethod.EditorInfo;
import android.webkit.CookieManager;
import android.webkit.DownloadListener;
import android.webkit.JavascriptInterface;
import android.webkit.URLUtil;
import android.webkit.ValueCallback;
import android.webkit.WebChromeClient;
import android.webkit.WebResourceError;
import android.webkit.WebResourceRequest;
import android.webkit.WebSettings;
import android.webkit.WebView;
import android.webkit.WebViewClient;
import android.widget.Button;
import android.widget.EditText;
import android.widget.FrameLayout;
import android.widget.LinearLayout;
import android.widget.ProgressBar;
import android.widget.ScrollView;
import android.widget.TextView;
import android.widget.Toast;

import java.io.InputStream;
import java.net.HttpURLConnection;
import java.net.URL;
import java.net.URLDecoder;
import java.util.regex.Matcher;
import java.util.regex.Pattern;

public class MainActivity extends Activity {
    static final String VERSION = "1.14.0";
    static final int DEFAULT_PORT = 18765;
    static final int REQ_FILE = 1;
    static final int REQ_STORAGE = 2;
    static final int BLUE = 0xFF1E2AB0;
    static final int INK = 0xFF14161F;
    static final int MUTED = 0xFF6B6A66;
    static final int BG = 0xFFF6F4EE;

    SharedPreferences prefs;
    String server;
    FrameLayout root;
    WebView web;
    ProgressBar bar;
    ScrollView setupView;
    LinearLayout errorView;
    TextView errorText;
    EditText addrInput;
    TextView setupStatus;
    Button connectBtn;
    ValueCallback<Uri[]> fileCallback;

    @Override
    protected void onCreate(Bundle state) {
        super.onCreate(state);
        prefs = getSharedPreferences("kyws", MODE_PRIVATE);
        server = prefs.getString("server", null);
        if (Build.VERSION.SDK_INT >= 21) getWindow().setStatusBarColor(BLUE);
        buildUi();
        if (!handleIntent(getIntent())) {
            if (server != null) loadServer();
            else showSetup(null);
        }
    }

    @Override
    protected void onNewIntent(Intent intent) {
        super.onNewIntent(intent);
        handleIntent(intent);
    }

    // 相机扫码后“用 CanDo 打开”：http://电脑IP:18765/...
    boolean handleIntent(Intent intent) {
        if (intent == null || intent.getData() == null) return false;
        Uri u = intent.getData();
        if (!"http".equals(u.getScheme()) || u.getHost() == null) return false;
        int port = u.getPort() > 0 ? u.getPort() : DEFAULT_PORT;
        showSetup(u.getHost() + ":" + port);
        connect(u.getHost() + ":" + port);
        return true;
    }

    int dp(int v) { return (int) (v * getResources().getDisplayMetrics().density + 0.5f); }

    TextView text(String s, int sizeSp, int color, boolean bold) {
        TextView t = new TextView(this);
        t.setText(s);
        t.setTextSize(sizeSp);
        t.setTextColor(color);
        t.setLineSpacing(dp(3), 1f);
        if (bold) t.setTypeface(Typeface.DEFAULT_BOLD);
        return t;
    }

    Button button(String s, boolean primary) {
        Button b = new Button(this);
        b.setText(s);
        b.setAllCaps(false);
        b.setTextSize(16);
        if (primary) {
            b.setTextColor(Color.WHITE);
            b.getBackground().setTint(BLUE);
        }
        return b;
    }

    LinearLayout.LayoutParams lp(int top) {
        LinearLayout.LayoutParams p = new LinearLayout.LayoutParams(ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT);
        p.topMargin = dp(top);
        return p;
    }

    void buildUi() {
        root = new FrameLayout(this);
        root.setBackgroundColor(BG);

        web = new WebView(this);
        root.addView(web, new FrameLayout.LayoutParams(-1, -1));
        setupWeb();

        bar = new ProgressBar(this, null, android.R.attr.progressBarStyleHorizontal);
        bar.setMax(100);
        bar.setVisibility(View.GONE);
        FrameLayout.LayoutParams bp = new FrameLayout.LayoutParams(-1, dp(4));
        bp.gravity = Gravity.TOP;
        root.addView(bar, bp);

        // ---- 首次设置：填写电脑地址 ----
        setupView = new ScrollView(this);
        setupView.setBackgroundColor(BG);
        LinearLayout s = new LinearLayout(this);
        s.setOrientation(LinearLayout.VERTICAL);
        s.setPadding(dp(24), dp(40), dp(24), dp(32));
        s.addView(text("CanDo", 26, INK, true));
        s.addView(text("连接老师电脑上的“CanDo 可为”。所有资料都保存在老师电脑上，手机和电脑需要连同一个 Wi-Fi。", 15, MUTED, false), lp(10));
        s.addView(text("方法一（推荐）：用手机相机或微信扫一扫，扫描电脑上“设置 → 手机与同学访问”里的二维码，选择用“CanDo”打开。", 15, INK, false), lp(22));
        s.addView(text("方法二：输入二维码下方显示的访问地址：", 15, INK, false), lp(16));
        addrInput = new EditText(this);
        addrInput.setHint("例如 192.168.1.5:18765");
        addrInput.setSingleLine(true);
        addrInput.setInputType(InputType.TYPE_CLASS_TEXT | InputType.TYPE_TEXT_VARIATION_URI);
        addrInput.setImeOptions(EditorInfo.IME_ACTION_GO);
        addrInput.setOnEditorActionListener(new TextView.OnEditorActionListener() {
            public boolean onEditorAction(TextView v, int id, KeyEvent e) {
                connect(addrInput.getText().toString());
                return true;
            }
        });
        s.addView(addrInput, lp(8));
        connectBtn = button("连接", true);
        connectBtn.setOnClickListener(new View.OnClickListener() {
            public void onClick(View v) { connect(addrInput.getText().toString()); }
        });
        s.addView(connectBtn, lp(12));
        setupStatus = text("", 14, MUTED, false);
        s.addView(setupStatus, lp(12));
        s.addView(text("连不上？\n① 确认电脑已开机，桌面上的“CanDo 可为”已打开；\n② 老师已在电脑上开启“手机与同学访问”；\n③ 手机和电脑连的是同一个 Wi-Fi（部分校园网禁止设备互访，可让电脑连接手机热点）。", 14, MUTED, false), lp(24));
        s.addView(text("版本 " + VERSION, 12, MUTED, false), lp(24));
        setupView.addView(s);
        root.addView(setupView, new FrameLayout.LayoutParams(-1, -1));

        // ---- 连接失败页 ----
        errorView = new LinearLayout(this);
        errorView.setOrientation(LinearLayout.VERTICAL);
        errorView.setGravity(Gravity.CENTER_VERTICAL);
        errorView.setPadding(dp(28), dp(28), dp(28), dp(28));
        errorView.setBackgroundColor(BG);
        errorView.addView(text("连接不上老师的电脑", 22, INK, true));
        errorText = text("", 15, MUTED, false);
        errorView.addView(errorText, lp(12));
        Button retry = button("重试", true);
        retry.setOnClickListener(new View.OnClickListener() {
            public void onClick(View v) { loadServer(); }
        });
        errorView.addView(retry, lp(24));
        Button change = button("更换电脑地址", false);
        change.setOnClickListener(new View.OnClickListener() {
            public void onClick(View v) { showSetup(server == null ? null : stripScheme(server)); }
        });
        errorView.addView(change, lp(10));
        root.addView(errorView, new FrameLayout.LayoutParams(-1, -1));

        setContentView(root);
    }

    void setupWeb() {
        WebSettings ws = web.getSettings();
        ws.setJavaScriptEnabled(true);
        ws.setDomStorageEnabled(true);
        ws.setDatabaseEnabled(true);
        ws.setAllowFileAccess(false);
        ws.setAllowContentAccess(true);
        ws.setLoadWithOverviewMode(true);
        ws.setUseWideViewPort(true);
        ws.setSupportMultipleWindows(false);
        ws.setUserAgentString(ws.getUserAgentString() + " KeyanApp/" + VERSION);
        CookieManager cm = CookieManager.getInstance();
        cm.setAcceptCookie(true);
        cm.setAcceptThirdPartyCookies(web, false);
        web.addJavascriptInterface(new Bridge(), "KyApp");

        web.setWebViewClient(new WebViewClient() {
            @Override
            public boolean shouldOverrideUrlLoading(WebView view, String url) {
                if (server != null && url.startsWith(server)) return false;
                openExternal(url);
                return true;
            }

            @Override
            public void onPageFinished(WebView view, String url) {
                bar.setVisibility(View.GONE);
                CookieManager.getInstance().flush();
            }

            @Override
            public void onReceivedError(WebView view, WebResourceRequest req, WebResourceError err) {
                if (req.isForMainFrame()) showError();
            }

            @SuppressWarnings("deprecation")
            @Override
            public void onReceivedError(WebView view, int code, String desc, String url) {
                if (Build.VERSION.SDK_INT < 23) showError();
            }
        });

        web.setWebChromeClient(new WebChromeClient() {
            @Override
            public void onProgressChanged(WebView view, int p) {
                bar.setProgress(p);
                bar.setVisibility(p < 100 ? View.VISIBLE : View.GONE);
            }

            // 网页中的“选择文件”（上传资料、读取论文）
            @Override
            public boolean onShowFileChooser(WebView view, ValueCallback<Uri[]> cb, FileChooserParams params) {
                if (fileCallback != null) fileCallback.onReceiveValue(null);
                fileCallback = cb;
                try {
                    Intent i = params.createIntent();
                    i.addCategory(Intent.CATEGORY_OPENABLE);
                    i.setType("*/*");
                    startActivityForResult(Intent.createChooser(i, "选择文件"), REQ_FILE);
                } catch (Exception e) {
                    fileCallback = null;
                    toast("无法打开文件选择器");
                    return false;
                }
                return true;
            }
        });

        // 原文件、备份等下载：带上登录状态交给系统下载管理器
        web.setDownloadListener(new DownloadListener() {
            public void onDownloadStart(String url, String ua, String disposition, String mime, long len) {
                download(url, ua, disposition, mime);
            }
        });
    }

    @Override
    protected void onActivityResult(int req, int res, Intent data) {
        if (req == REQ_FILE && fileCallback != null) {
            Uri[] result = WebChromeClient.FileChooserParams.parseResult(res, data);
            fileCallback.onReceiveValue(result);
            fileCallback = null;
            return;
        }
        super.onActivityResult(req, res, data);
    }

    String pendingDl[];

    void download(String url, String ua, String disposition, String mime) {
        if (Build.VERSION.SDK_INT >= 23 && Build.VERSION.SDK_INT < 29
                && checkSelfPermission(Manifest.permission.WRITE_EXTERNAL_STORAGE) != PackageManager.PERMISSION_GRANTED) {
            pendingDl = new String[]{url, ua, disposition, mime};
            requestPermissions(new String[]{Manifest.permission.WRITE_EXTERNAL_STORAGE}, REQ_STORAGE);
            return;
        }
        try {
            String name = fileName(url, disposition, mime);
            DownloadManager.Request r = new DownloadManager.Request(Uri.parse(url));
            String cookie = CookieManager.getInstance().getCookie(url);
            if (cookie != null) r.addRequestHeader("Cookie", cookie);
            r.addRequestHeader("User-Agent", ua);
            if (mime != null) r.setMimeType(mime);
            r.setTitle(name);
            r.setNotificationVisibility(DownloadManager.Request.VISIBILITY_VISIBLE_NOTIFY_COMPLETED);
            r.setDestinationInExternalPublicDir(Environment.DIRECTORY_DOWNLOADS, name);
            DownloadManager dm = (DownloadManager) getSystemService(Context.DOWNLOAD_SERVICE);
            dm.enqueue(r);
            toast("开始下载：" + name + "\n完成后可在通知栏或“下载”文件夹中打开");
        } catch (Exception e) {
            toast("下载失败：" + e.getMessage());
        }
    }

    @Override
    public void onRequestPermissionsResult(int req, String[] perms, int[] results) {
        if (req == REQ_STORAGE && pendingDl != null) {
            String[] d = pendingDl;
            pendingDl = null;
            if (results.length > 0 && results[0] == PackageManager.PERMISSION_GRANTED) download(d[0], d[1], d[2], d[3]);
            else toast("没有存储权限，无法保存文件");
        }
    }

    static String fileName(String url, String disposition, String mime) {
        if (disposition != null) {
            Matcher m = Pattern.compile("filename\\*=UTF-8''([^;]+)", Pattern.CASE_INSENSITIVE).matcher(disposition);
            if (m.find()) {
                try {
                    return URLDecoder.decode(m.group(1).trim(), "UTF-8").replaceAll("[\\\\/:*?\"<>|]", "_");
                } catch (Exception ignored) {
                }
            }
        }
        return URLUtil.guessFileName(url, disposition, mime);
    }

    void openExternal(String url) {
        try {
            startActivity(new Intent(Intent.ACTION_VIEW, Uri.parse(url)));
        } catch (Exception e) {
            toast("无法打开链接");
        }
    }

    static String stripScheme(String s) {
        return s.replaceFirst("^https?://", "");
    }

    // 输入 “192.168.1.5” / “192.168.1.5:18765” / “http://192.168.1.5:18765/” 都可以
    static String normalize(String in) {
        String s = in.trim().replace("：", ":").replace("。", ".");
        if (s.isEmpty()) return null;
        if (!s.startsWith("http://") && !s.startsWith("https://")) s = "http://" + s;
        try {
            Uri u = Uri.parse(s);
            if (u.getHost() == null || u.getHost().isEmpty()) return null;
            int port = u.getPort() > 0 ? u.getPort() : DEFAULT_PORT;
            return u.getScheme() + "://" + u.getHost() + ":" + port;
        } catch (Exception e) {
            return null;
        }
    }

    void connect(String input) {
        final String target = normalize(input);
        if (target == null) {
            setupStatus.setTextColor(0xFFB42318);
            setupStatus.setText("请输入电脑上显示的访问地址，例如 192.168.1.5:18765");
            return;
        }
        setupStatus.setTextColor(MUTED);
        setupStatus.setText("正在连接 " + stripScheme(target) + " …");
        connectBtn.setEnabled(false);
        new Thread(new Runnable() {
            public void run() {
                final String err = check(target);
                runOnUiThread(new Runnable() {
                    public void run() {
                        connectBtn.setEnabled(true);
                        if (err == null) {
                            server = target;
                            prefs.edit().putString("server", server).apply();
                            setupStatus.setText("");
                            loadServer();
                        } else {
                            setupStatus.setTextColor(0xFFB42318);
                            setupStatus.setText(err);
                        }
                    }
                });
            }
        }).start();
    }

    // 检查地址是否为 CanDo 可为；返回 null 表示成功
    static String check(String base) {
        HttpURLConnection c = null;
        try {
            c = (HttpURLConnection) new URL(base + "/api/health").openConnection();
            c.setConnectTimeout(5000);
            c.setReadTimeout(5000);
            int code = c.getResponseCode();
            if (code == 403) return "已连上电脑，但老师还没有开启“手机与同学访问”。请在电脑上：设置 → 手机与同学访问 → 勾选“允许手机和同学访问”。";
            if (code != 200) return "连接失败（状态 " + code + "）。请确认地址是否正确。";
            InputStream in = c.getInputStream();
            byte[] buf = new byte[4096];
            int n = in.read(buf);
            String body = n > 0 ? new String(buf, 0, n, "UTF-8") : "";
            if (!body.contains("\"app\":\"kyws\"")) return "这个地址不是 CanDo 可为，请检查地址。";
            return null;
        } catch (java.net.SocketTimeoutException e) {
            return "连接超时。请确认手机和电脑连的是同一个 Wi-Fi，且电脑上的工作台已打开。";
        } catch (Exception e) {
            return "连接不上 " + stripScheme(base) + "。请确认电脑已开机、工作台已打开，且手机和电脑在同一个 Wi-Fi 下。";
        } finally {
            if (c != null) c.disconnect();
        }
    }

    void loadServer() {
        if (server == null) {
            showSetup(null);
            return;
        }
        setupView.setVisibility(View.GONE);
        errorView.setVisibility(View.GONE);
        web.setVisibility(View.VISIBLE);
        String cur = web.getUrl();
        if (cur != null && cur.startsWith(server) && !cur.startsWith("about:")) web.reload();
        else web.loadUrl(server + "/");
    }

    void showSetup(String prefill) {
        web.setVisibility(View.INVISIBLE);
        errorView.setVisibility(View.GONE);
        setupView.setVisibility(View.VISIBLE);
        if (prefill != null) addrInput.setText(prefill);
    }

    void showError() {
        bar.setVisibility(View.GONE);
        web.setVisibility(View.INVISIBLE);
        setupView.setVisibility(View.GONE);
        errorText.setText("当前连接的电脑：" + (server == null ? "未设置" : stripScheme(server))
                + "\n\n请确认：\n① 电脑已开机，“CanDo 可为”已打开；\n② 电脑上已开启“手机与同学访问”；\n③ 手机和电脑连的是同一个 Wi-Fi。\n\n如果电脑换了网络，地址可能会变，请点“更换电脑地址”重新输入或扫码。");
        errorView.setVisibility(View.VISIBLE);
    }

    void toast(String s) {
        Toast.makeText(this, s, Toast.LENGTH_LONG).show();
    }

    @Override
    public void onBackPressed() {
        if (setupView.getVisibility() == View.VISIBLE && server != null) {
            loadServer();
            return;
        }
        if (web.getVisibility() == View.VISIBLE && web.canGoBack()) {
            web.goBack();
            return;
        }
        super.onBackPressed();
    }

    @Override
    protected void onPause() {
        super.onPause();
        CookieManager.getInstance().flush();
    }

    // 网页中调用：window.KyApp.xxx()
    class Bridge {
        @JavascriptInterface
        public String server() { return server == null ? "" : stripScheme(server); }

        @JavascriptInterface
        public String version() { return VERSION; }

        @JavascriptInterface
        public void openExternal(final String url) {
            if (url == null || !(url.startsWith("https://") || url.startsWith("http://"))) return;
            runOnUiThread(new Runnable() {
                @Override
                public void run() { MainActivity.this.openExternal(url); }
            });
        }

        @JavascriptInterface
        public void changeServer() {
            runOnUiThread(new Runnable() {
                public void run() { showSetup(server == null ? null : stripScheme(server)); }
            });
        }
    }
}
