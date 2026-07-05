import path from 'path'

function requireCachedOhosPackage(packageName: string) {
    const hvigorSegment = `${path.sep}@ohos${path.sep}hvigor${path.sep}`;
    const hvigorModule = Object.keys(require.cache)
        .map((it) => path.normalize(it))
        .find((it) => it.includes(hvigorSegment));
    if (!hvigorModule) {
        return require(packageName);
    }
    const workspaceNodeModules = hvigorModule.split(hvigorSegment)[0];
    return require(path.join(workspaceNodeModules, '@ohos', packageName.split('/')[1]));
}

const { appTasks } = requireCachedOhosPackage('@ohos/hvigor-ohos-plugin');
const { flutterHvigorPlugin } = require(path.join(__dirname, 'node_modules', 'flutter-hvigor-plugin'));

export default {
    system: appTasks,  /* Built-in plugin of Hvigor. It cannot be modified. */
    plugins:[flutterHvigorPlugin(path.dirname(__dirname))]         /* Custom plugin to extend the functionality of Hvigor. */
}
