
syncwjt:
	@read -p "提示:同步操作尽量在完成一个完整功能特性后进行，请输入提交描述 (wjt):  " cimsg; \
	git commit -am "$(shell date "+%F %T") : $${cimsg}" || echo "no commit"
	# 切换主分支并更新
	git checkout develop
	git pull origin develop
	# 切换开发分支变基合并提交
	git checkout wjt
	git rebase -i develop
	# 切换回主分支并合并开发者分支，推送主分支到远程，方便其他开发者合并
	git checkout develop
	git merge --no-ff wjt
	git push origin develop
	# 切换回自己的开发分支继续工作
	git checkout wjt


